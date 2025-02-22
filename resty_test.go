// Copyright (c) 2015-2021 Jeevanandam M (jeeva@myjeeva.com), All rights reserved.
// resty source code and usage is governed by a MIT style
// license that can be found in the LICENSE file.

package resty

import (
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

//‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾
// Testing Unexported methods
//___________________________________

func getTestDataPath() string {
	pwd, _ := os.Getwd()
	return filepath.Join(pwd, ".testdata")
}

func createGetServer(t *testing.T) *httptest.Server {
	var attempt int32
	var sequence int32
	var lastRequest time.Time
	ts := createTestServer(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("Method: %v", r.Method)
		t.Logf("Path: %v", r.URL.Path)

		if r.Method == MethodGet {
			switch r.URL.Path {
			case "/":
				_, _ = w.Write([]byte("TestGet: text response"))
			case "/no-content":
				_, _ = w.Write([]byte(""))
			case "/json":
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"TestGet": "JSON response"}`))
			case "/json-invalid":
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte("TestGet: Invalid JSON"))
			case "/long-text":
				_, _ = w.Write([]byte("TestGet: text response with size > 30"))
			case "/long-json":
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"TestGet": "JSON response with size > 30"}`))
			case "/mypage":
				w.WriteHeader(http.StatusBadRequest)
			case "/mypage2":
				_, _ = w.Write([]byte("TestGet: text response from mypage2"))
			case "/set-retrycount-test":
				attp := atomic.AddInt32(&attempt, 1)
				if attp <= 4 {
					time.Sleep(time.Second * 6)
				}
				_, _ = w.Write([]byte("TestClientRetry page"))
			case "/set-retrywaittime-test":
				// Returns time.Duration since last request here
				// or 0 for the very first request
				if atomic.LoadInt32(&attempt) == 0 {
					lastRequest = time.Now()
					_, _ = fmt.Fprint(w, "0")
				} else {
					now := time.Now()
					sinceLastRequest := now.Sub(lastRequest)
					lastRequest = now
					_, _ = fmt.Fprintf(w, "%d", uint64(sinceLastRequest))
				}
				atomic.AddInt32(&attempt, 1)

			case "/set-retry-error-recover":
				w.Header().Set(hdrContentTypeKey, "application/json; charset=utf-8")
				if atomic.LoadInt32(&attempt) == 0 {
					w.WriteHeader(http.StatusTooManyRequests)
					_, _ = w.Write([]byte(`{ "message": "too many" }`))
				} else {
					_, _ = w.Write([]byte(`{ "message": "hello" }`))
				}
				atomic.AddInt32(&attempt, 1)
			case "/set-timeout-test-with-sequence":
				seq := atomic.AddInt32(&sequence, 1)
				time.Sleep(time.Second * 2)
				_, _ = fmt.Fprintf(w, "%d", seq)
			case "/set-timeout-test":
				time.Sleep(time.Second * 6)
				_, _ = w.Write([]byte("TestClientTimeout page"))
			case "/my-image.png":
				fileBytes, _ := ioutil.ReadFile(filepath.Join(getTestDataPath(), "test-img.png"))
				w.Header().Set("Content-Type", "image/png")
				w.Header().Set("Content-Length", strconv.Itoa(len(fileBytes)))
				_, _ = w.Write(fileBytes)
			case "/get-method-payload-test":
				body, err := ioutil.ReadAll(r.Body)
				if err != nil {
					t.Errorf("Error: could not read get body: %s", err.Error())
				}
				_, _ = w.Write(body)
			case "/host-header":
				_, _ = w.Write([]byte(r.Host))
			}

			switch {
			case strings.HasPrefix(r.URL.Path, "/v1/users/sample@sample.com/100002"):
				if strings.HasSuffix(r.URL.Path, "details") {
					_, _ = w.Write([]byte("TestGetPathParams: text response: " + r.URL.String()))
				} else {
					_, _ = w.Write([]byte("TestPathParamURLInput: text response: " + r.URL.String()))
				}
			}

		}
	})

	return ts
}

func handleLoginEndpoint(t *testing.T, w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/login" {
		user := &User{}

		// JSON
		if IsJSONType(r.Header.Get(hdrContentTypeKey)) {
			jd := json.NewDecoder(r.Body)
			err := jd.Decode(user)
			if r.URL.Query().Get("ct") == "problem" {
				w.Header().Set(hdrContentTypeKey, "application/problem+json; charset=utf-8")
			} else if r.URL.Query().Get("ct") == "rpc" {
				w.Header().Set(hdrContentTypeKey, "application/json-rpc")
			} else {
				w.Header().Set(hdrContentTypeKey, "application/json")
			}

			if err != nil {
				t.Logf("Error: %#v", err)
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{ "id": "bad_request", "message": "Unable to read user info" }`))
				return
			}

			if user.Username == "testuser" && user.Password == "testpass" {
				_, _ = w.Write([]byte(`{ "id": "success", "message": "login successful" }`))
			} else if user.Username == "testuser" && user.Password == "invalidjson" {
				_, _ = w.Write([]byte(`{ "id": "success", "message": "login successful", }`))
			} else {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{ "id": "unauthorized", "message": "Invalid credentials" }`))
			}

			return
		}

		// XML
		if IsXMLType(r.Header.Get(hdrContentTypeKey)) {
			xd := xml.NewDecoder(r.Body)
			err := xd.Decode(user)

			w.Header().Set(hdrContentTypeKey, "application/xml")
			if err != nil {
				t.Logf("Error: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>`))
				_, _ = w.Write([]byte(`<AuthError><Id>bad_request</Id><Message>Unable to read user info</Message></AuthError>`))
				return
			}

			if user.Username == "testuser" && user.Password == "testpass" {
				_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>`))
				_, _ = w.Write([]byte(`<AuthSuccess><Id>success</Id><Message>login successful</Message></AuthSuccess>`))
			} else if user.Username == "testuser" && user.Password == "invalidxml" {
				_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>`))
				_, _ = w.Write([]byte(`<AuthSuccess><Id>success</Id><Message>login successful</AuthSuccess>`))
			} else {
				w.Header().Set("Www-Authenticate", "Protected Realm")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>`))
				_, _ = w.Write([]byte(`<AuthError><Id>unauthorized</Id><Message>Invalid credentials</Message></AuthError>`))
			}

			return
		}
	}
}

func handleUsersEndpoint(t *testing.T, w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/users" {
		// JSON
		if IsJSONType(r.Header.Get(hdrContentTypeKey)) {
			var users []ExampleUser
			jd := json.NewDecoder(r.Body)
			err := jd.Decode(&users)
			w.Header().Set(hdrContentTypeKey, "application/json")
			if err != nil {
				t.Logf("Error: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{ "id": "bad_request", "message": "Unable to read user info" }`))
				return
			}

			// logic check, since we are excepting to reach 3 records
			if len(users) != 3 {
				t.Log("Error: Excepted count of 3 records")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{ "id": "bad_request", "message": "Expected record count doesn't match" }`))
				return
			}

			eu := users[2]
			if eu.FirstName == "firstname3" && eu.ZipCode == "10003" {
				w.WriteHeader(http.StatusAccepted)
				_, _ = w.Write([]byte(`{ "message": "Accepted" }`))
			}

			return
		}
	}
}

func createPostServer(t *testing.T) *httptest.Server {
	ts := createTestServer(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("Method: %v", r.Method)
		t.Logf("Path: %v", r.URL.Path)
		t.Logf("RawQuery: %v", r.URL.RawQuery)
		t.Logf("Content-Type: %v", r.Header.Get(hdrContentTypeKey))

		if r.Method == MethodPost {
			handleLoginEndpoint(t, w, r)

			handleUsersEndpoint(t, w, r)

			if r.URL.Path == "/login-json-html" {
				w.Header().Set(hdrContentTypeKey, "text/html")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`<htm><body>Test JSON request with HTML response</body></html>`))
				return
			}

			if r.URL.Path == "/usersmap" {
				// JSON
				if IsJSONType(r.Header.Get(hdrContentTypeKey)) {
					if r.URL.Query().Get("status") == "500" {
						body, err := ioutil.ReadAll(r.Body)
						if err != nil {
							t.Errorf("Error: could not read post body: %s", err.Error())
						}
						t.Logf("Got query param: status=500 so we're returning the post body as response and a 500 status code. body: %s", string(body))
						w.Header().Set(hdrContentTypeKey, "application/json; charset=utf-8")
						w.WriteHeader(http.StatusInternalServerError)
						_, _ = w.Write(body)
						return
					}

					var users []map[string]interface{}
					jd := json.NewDecoder(r.Body)
					err := jd.Decode(&users)
					w.Header().Set(hdrContentTypeKey, "application/json; charset=utf-8")
					if err != nil {
						t.Logf("Error: %v", err)
						w.WriteHeader(http.StatusBadRequest)
						_, _ = w.Write([]byte(`{ "id": "bad_request", "message": "Unable to read user info" }`))
						return
					}

					// logic check, since we are excepting to reach 1 map records
					if len(users) != 1 {
						t.Log("Error: Excepted count of 1 map records")
						w.WriteHeader(http.StatusBadRequest)
						_, _ = w.Write([]byte(`{ "id": "bad_request", "message": "Expected record count doesn't match" }`))
						return
					}

					w.WriteHeader(http.StatusAccepted)
					_, _ = w.Write([]byte(`{ "message": "Accepted" }`))

					return
				}
			} else if r.URL.Path == "/redirect" {
				w.Header().Set(hdrLocationKey, "/login")
				w.WriteHeader(http.StatusTemporaryRedirect)
			}
		}
	})

	return ts
}

func createFormPostServer(t *testing.T) *httptest.Server {
	ts := createTestServer(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("Method: %v", r.Method)
		t.Logf("Path: %v", r.URL.Path)
		t.Logf("Content-Type: %v", r.Header.Get(hdrContentTypeKey))

		if r.Method == MethodPost {
			_ = r.ParseMultipartForm(10e6)

			if r.URL.Path == "/profile" {
				t.Logf("FirstName: %v", r.FormValue("first_name"))
				t.Logf("LastName: %v", r.FormValue("last_name"))
				t.Logf("City: %v", r.FormValue("city"))
				t.Logf("Zip Code: %v", r.FormValue("zip_code"))

				_, _ = w.Write([]byte("Success"))
				return
			} else if r.URL.Path == "/search" {
				formEncodedData := r.Form.Encode()
				t.Logf("Received Form Encoded values: %v", formEncodedData)

				assertEqual(t, true, strings.Contains(formEncodedData, "search_criteria=pencil"))
				assertEqual(t, true, strings.Contains(formEncodedData, "search_criteria=glass"))

				_, _ = w.Write([]byte("Success"))
				return
			} else if r.URL.Path == "/upload" {
				t.Logf("FirstName: %v", r.FormValue("first_name"))
				t.Logf("LastName: %v", r.FormValue("last_name"))

				targetPath := filepath.Join(getTestDataPath(), "upload")
				_ = os.MkdirAll(targetPath, 0700)

				for _, fhdrs := range r.MultipartForm.File {
					for _, hdr := range fhdrs {
						t.Logf("Name: %v", hdr.Filename)
						t.Logf("Header: %v", hdr.Header)
						dotPos := strings.LastIndex(hdr.Filename, ".")

						fname := fmt.Sprintf("%s-%v%s", hdr.Filename[:dotPos], time.Now().Unix(), hdr.Filename[dotPos:])
						t.Logf("Write name: %v", fname)

						infile, _ := hdr.Open()
						f, err := os.OpenFile(filepath.Join(targetPath, fname), os.O_WRONLY|os.O_CREATE, 0666)
						if err != nil {
							t.Logf("Error: %v", err)
							return
						}
						defer func() {
							_ = f.Close()
						}()
						_, _ = io.Copy(f, infile)

						_, _ = w.Write([]byte(fmt.Sprintf("File: %v, uploaded as: %v\n", hdr.Filename, fname)))
					}
				}

				return
			}
		}
	})

	return ts
}

func createFilePostServer(t *testing.T) *httptest.Server {
	ts := createTestServer(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("Method: %v", r.Method)
		t.Logf("Path: %v", r.URL.Path)
		t.Logf("Content-Type: %v", r.Header.Get(hdrContentTypeKey))

		if r.Method != MethodPost {
			t.Log("createPostServer:: Not a Post request")
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, http.StatusText(http.StatusBadRequest))
			return
		}

		targetPath := filepath.Join(getTestDataPath(), "upload-large")
		_ = os.MkdirAll(targetPath, 0700)
		defer cleanupFiles(targetPath)

		switch r.URL.Path {
		case "/upload":
			f, err := os.OpenFile(filepath.Join(targetPath, "large-file.png"),
				os.O_WRONLY|os.O_CREATE, 0666)
			if err != nil {
				t.Logf("Error: %v", err)
				return
			}
			defer func() {
				_ = f.Close()
			}()
			size, _ := io.Copy(f, r.Body)

			fmt.Fprintf(w, "File Uploaded successfully, file size: %v", size)
		case "/set-reset-multipart-readers-test":
			w.Header().Set(hdrContentTypeKey, "application/json; charset=utf-8")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprintf(w, `{ "message": "error" }`)
		}
	})

	return ts
}

func createAuthServer(t *testing.T) *httptest.Server {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("Method: %v", r.Method)
		t.Logf("Path: %v", r.URL.Path)
		t.Logf("Content-Type: %v", r.Header.Get(hdrContentTypeKey))

		if r.Method == MethodGet {
			if r.URL.Path == "/profile" {
				// 004DDB79-6801-4587-B976-F093E6AC44FF
				auth := r.Header.Get("Authorization")
				t.Logf("Bearer Auth: %v", auth)

				w.Header().Set(hdrContentTypeKey, "application/json; charset=utf-8")

				if !strings.HasPrefix(auth, "Bearer ") {
					w.Header().Set("Www-Authenticate", "Protected Realm")
					w.WriteHeader(http.StatusUnauthorized)
					_, _ = w.Write([]byte(`{ "id": "unauthorized", "message": "Invalid credentials" }`))

					return
				}

				if auth[7:] == "004DDB79-6801-4587-B976-F093E6AC44FF" || auth[7:] == "004DDB79-6801-4587-B976-F093E6AC44FF-Request" {
					_, _ = w.Write([]byte(`{ "id": "success", "message": "login successful" }`))
				}
			}

			return
		}

		if r.Method == MethodPost {
			if r.URL.Path == "/login" {
				auth := r.Header.Get("Authorization")
				t.Logf("Basic Auth: %v", auth)

				w.Header().Set(hdrContentTypeKey, "application/json; charset=utf-8")

				password, err := base64.StdEncoding.DecodeString(auth[6:])
				if err != nil || string(password) != "myuser:basicauth" {
					w.Header().Set("Www-Authenticate", "Protected Realm")
					w.WriteHeader(http.StatusUnauthorized)
					_, _ = w.Write([]byte(`{ "id": "unauthorized", "message": "Invalid credentials" }`))

					return
				}

				_, _ = w.Write([]byte(`{ "id": "success", "message": "login successful" }`))
			}

			return
		}
	}))

	return ts
}

func createGenServer(t *testing.T) *httptest.Server {
	ts := createTestServer(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("Method: %v", r.Method)
		t.Logf("Path: %v", r.URL.Path)

		if r.Method == MethodGet {
			if r.URL.Path == "/json-no-set" {
				// Set empty header value for testing, since Go server sets to
				// text/plain; charset=utf-8
				w.Header().Set(hdrContentTypeKey, "")
				_, _ = w.Write([]byte(`{"response":"json response no content type set"}`))
			} else if r.URL.Path == "/gzip-test" {
				w.Header().Set(hdrContentTypeKey, plainTextType)
				w.Header().Set(hdrContentEncodingKey, "gzip")
				zw := gzip.NewWriter(w)
				_, _ = zw.Write([]byte("This is Gzip response testing"))
				zw.Close()
			} else if r.URL.Path == "/gzip-test-gziped-empty-body" {
				w.Header().Set(hdrContentTypeKey, plainTextType)
				w.Header().Set(hdrContentEncodingKey, "gzip")
				zw := gzip.NewWriter(w)
				// write gziped empty body
				_, _ = zw.Write([]byte(""))
				zw.Close()
			} else if r.URL.Path == "/gzip-test-no-gziped-body" {
				w.Header().Set(hdrContentTypeKey, plainTextType)
				w.Header().Set(hdrContentEncodingKey, "gzip")
				// don't write body
			}

			return
		}

		if r.Method == MethodPut {
			if r.URL.Path == "/plaintext" {
				_, _ = w.Write([]byte("TestPut: plain text response"))
			} else if r.URL.Path == "/json" {
				w.Header().Set(hdrContentTypeKey, "application/json; charset=utf-8")
				_, _ = w.Write([]byte(`{"response":"json response"}`))
			} else if r.URL.Path == "/xml" {
				w.Header().Set(hdrContentTypeKey, "application/xml")
				_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><Response>XML response</Response>`))
			}
			return
		}

		if r.Method == MethodOptions && r.URL.Path == "/options" {
			w.Header().Set("Access-Control-Allow-Origin", "localhost")
			w.Header().Set("Access-Control-Allow-Methods", "PUT, PATCH")
			w.Header().Set("Access-Control-Expose-Headers", "x-go-resty-id")
			w.WriteHeader(http.StatusOK)
			return
		}

		if r.Method == MethodPatch && r.URL.Path == "/patch" {
			w.WriteHeader(http.StatusOK)
			return
		}

		if r.Method == "REPORT" && r.URL.Path == "/report" {
			body, _ := ioutil.ReadAll(r.Body)
			if len(body) == 0 {
				w.WriteHeader(http.StatusOK)
			}
			return
		}
	})

	return ts
}

func createRedirectServer(t *testing.T) *httptest.Server {
	ts := createTestServer(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("Method: %v", r.Method)
		t.Logf("Path: %v", r.URL.Path)

		if r.Method == MethodGet {
			if strings.HasPrefix(r.URL.Path, "/redirect-host-check-") {
				cntStr := strings.SplitAfter(r.URL.Path, "-")[3]
				cnt, _ := strconv.Atoi(cntStr)

				if cnt != 7 { // Testing hard stop via logical
					if cnt >= 5 {
						http.Redirect(w, r, "http://httpbin.org/get", http.StatusTemporaryRedirect)
					} else {
						http.Redirect(w, r, fmt.Sprintf("/redirect-host-check-%d", cnt+1), http.StatusTemporaryRedirect)
					}
				}
			} else if strings.HasPrefix(r.URL.Path, "/redirect-") {
				cntStr := strings.SplitAfter(r.URL.Path, "-")[1]
				cnt, _ := strconv.Atoi(cntStr)

				http.Redirect(w, r, fmt.Sprintf("/redirect-%d", cnt+1), http.StatusTemporaryRedirect)
			}
		}
	})

	return ts
}

func createTestServer(fn func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(fn))
}

func dc() *Client {
	c := New().
		outputLogTo(ioutil.Discard)
	return c
}

func dcl() *Client {
	c := New().
		SetDebug(true).
		outputLogTo(ioutil.Discard)
	return c
}

func dcr() *Request {
	return dc().R()
}

func dclr() *Request {
	c := dc().
		SetDebug(true).
		outputLogTo(ioutil.Discard)
	return c.R()
}

func assertNil(t *testing.T, v interface{}) {
	if !isNil(v) {
		t.Errorf("[%v] was expected to be nil", v)
	}
}

func assertNotNil(t *testing.T, v interface{}) {
	if isNil(v) {
		t.Errorf("[%v] was expected to be non-nil", v)
	}
}

func assertType(t *testing.T, typ, v interface{}) {
	if reflect.DeepEqual(reflect.TypeOf(typ), reflect.TypeOf(v)) {
		t.Errorf("Expected type %t, got %t", typ, v)
	}
}

func assertError(t *testing.T, err error) {
	if err != nil {
		t.Errorf("Error occurred [%v]", err)
	}
}

func assertEqual(t *testing.T, e, g interface{}) (r bool) {
	if !equal(e, g) {
		t.Errorf("Expected [%v], got [%v]", e, g)
	}

	return
}

func assertNotEqual(t *testing.T, e, g interface{}) (r bool) {
	if equal(e, g) {
		t.Errorf("Expected [%v], got [%v]", e, g)
	} else {
		r = true
	}

	return
}

func equal(expected, got interface{}) bool {
	return reflect.DeepEqual(expected, got)
}

func isNil(v interface{}) bool {
	if v == nil {
		return true
	}

	rv := reflect.ValueOf(v)
	kind := rv.Kind()
	if kind >= reflect.Chan && kind <= reflect.Slice && rv.IsNil() {
		return true
	}

	return false
}

func logResponse(t *testing.T, resp *Response) {
	t.Logf("Response Status: %v", resp.Status())
	t.Logf("Response Time: %v", resp.Time())
	t.Logf("Response Headers: %v", resp.Header())
	t.Logf("Response Cookies: %v", resp.Cookies())
	t.Logf("Response Body: %v", resp)
}

func cleanupFiles(files ...string) {
	pwd, _ := os.Getwd()

	for _, f := range files {
		if filepath.IsAbs(f) {
			_ = os.RemoveAll(f)
		} else {
			_ = os.RemoveAll(filepath.Join(pwd, f))
		}
	}
}
