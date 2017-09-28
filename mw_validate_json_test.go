package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/TykTechnologies/tyk/user"
	"github.com/justinas/alice"
)

var schema = `{
    "title": "Person",
    "type": "object",
    "properties": {
        "firstName": {
            "type": "string"
        },
        "lastName": {
            "type": "string"
        },
        "age": {
            "description": "Age in years",
            "type": "integer",
            "minimum": 0
        }
    },
    "required": ["firstName", "lastName"]
}`

const validateJSONPathGatewaySetup = `{
	"api_id": "jsontest",
	"definition": {
		"location": "header",
		"key": "version"
	},
	"auth": {"auth_header_name": "authorization"},
	"version_data": {
		"not_versioned": true,
		"versions": {
			"default": {
				"name": "default",
				"use_extended_paths": true,
				"extended_paths": {
					"validate_json": [{
						"method": "POST",
						"path": "me",
						"validate_with_64": "ew0KICAgICJ0aXRsZSI6ICJQZXJzb24iLA0KICAgICJ0eXBlIjogIm9iamVjdCIsDQogICAgInByb3BlcnRpZXMiOiB7DQogICAgICAgICJmaXJzdE5hbWUiOiB7DQogICAgICAgICAgICAidHlwZSI6ICJzdHJpbmciDQogICAgICAgIH0sDQogICAgICAgICJsYXN0TmFtZSI6IHsNCiAgICAgICAgICAgICJ0eXBlIjogInN0cmluZyINCiAgICAgICAgfSwNCiAgICAgICAgImFnZSI6IHsNCiAgICAgICAgICAgICJkZXNjcmlwdGlvbiI6ICJBZ2UgaW4geWVhcnMiLA0KICAgICAgICAgICAgInR5cGUiOiAiaW50ZWdlciIsDQogICAgICAgICAgICAibWluaW11bSI6IDANCiAgICAgICAgfQ0KICAgIH0sDQogICAgInJlcXVpcmVkIjogWyJmaXJzdE5hbWUiLCAibGFzdE5hbWUiXQ0KfQ=="
					}]
				}
			}
		}
	},
	"proxy": {
		"listen_path": "/validate/",
		"target_url": "` + testHttpAny + `"
	}
}`

type res struct {
	Error string
	Code  int
}

type fixture struct {
	in   string
	out  res
	name string
}

var fixtures = []fixture{
	{
		in:   `{"age":23, "firstName": "Harry"}`,
		out:  res{Error: `lastName: lastName is required`, Code: http.StatusUnprocessableEntity},
		name: "missing field",
	},
	{
		in:   `{"age":23}`,
		out:  res{Error: `firstName: firstName is required, lastName: lastName is required`, Code: http.StatusUnprocessableEntity},
		name: "missing two fields",
	},
	{
		in:   `{}`,
		out:  res{Error: `firstName: firstName is required, lastName: lastName is required`, Code: http.StatusUnprocessableEntity},
		name: "empty object",
	},
	{
		in:   `[]`,
		out:  res{Error: `(root): Invalid type. Expected: object, given: array`, Code: http.StatusUnprocessableEntity},
		name: "array",
	},
	{
		in:   `{"age":"23", "firstName": "Harry", "lastName": "Potter"}`,
		out:  res{Error: `age: Invalid type. Expected: integer, given: string`, Code: http.StatusUnprocessableEntity},
		name: "wrong type",
	},
	{
		in:   `{"age":23, "firstName": "Harry", "lastName": "Potter"}`,
		out:  res{Error: `null`, Code: http.StatusOK},
		name: "valid",
	},
}

func TestValidateJSON_validate(t *testing.T) {

	for _, f := range fixtures {

		t.Run(f.name, func(st *testing.T) {
			vj := ValidateJSON{}

			res, _ := vj.validate([]byte(f.in), []byte(schema))

			st.Log("in:", f.in)
			if !res.Valid() && f.out.Code != http.StatusUnprocessableEntity {
				st.Error("Expected invalid")
			}

			if res.Valid() && f.out.Code != http.StatusOK {
				st.Error("expected valid")
			}
		})
	}
}

func TestValidateJSON_ProcessRequest(t *testing.T) {

	for _, f := range fixtures {

		t.Run(f.name, func(st *testing.T) {

			spec := createSpecTest(st, validateJSONPathGatewaySetup)
			recorder := httptest.NewRecorder()
			req := testReq(t, "POST", "/validate/me", f.in)

			session := createJSONVersionedSession()
			spec.SessionManager.UpdateSession("986968696869688869696999", session, 60)
			req.Header.Set("Authorization", "986968696869688869696999")

			chain := getJSONValidChain(spec)
			chain.ServeHTTP(recorder, req)

			if recorder.Code != f.out.Code {
				st.Errorf("failed: %v, code: %v (body: %v)", req.URL.String(), recorder.Code, recorder.Body)
			}

			if f.out.Code == http.StatusUnprocessableEntity {
				recorderBody := recorder.Body.String()
				if !strings.Contains(recorderBody, f.out.Error) {
					st.Errorf("Incorrect error msg:\nwant: %v\ngot: %v", f.out.Error, recorderBody)
				}
			}
		})
	}
}

func createJSONVersionedSession() *user.SessionState {
	session := new(user.SessionState)
	session.Rate = 10000
	session.Allowance = session.Rate
	session.LastCheck = time.Now().Unix()
	session.Per = 60
	session.Expires = -1
	session.QuotaRenewalRate = 300 // 5 minutes
	session.QuotaRenews = time.Now().Unix()
	session.QuotaRemaining = 10
	session.QuotaMax = -1
	session.AccessRights = map[string]user.AccessDefinition{"jsontest": {APIName: "Tyk Test API", APIID: "jsontest", Versions: []string{"default"}}}
	return session
}

func getJSONValidChain(spec *APISpec) http.Handler {
	remote, _ := url.Parse(spec.Proxy.TargetURL)
	proxy := TykNewSingleHostReverseProxy(remote, spec)
	proxyHandler := ProxyHandler(proxy, spec)
	baseMid := BaseMiddleware{spec, proxy}
	chain := alice.New(mwList(
		&IPWhiteListMiddleware{baseMid},
		&MiddlewareContextVars{BaseMiddleware: baseMid},
		&AuthKey{baseMid},
		&VersionCheck{BaseMiddleware: baseMid},
		&KeyExpired{baseMid},
		&AccessRightsCheck{baseMid},
		&RateLimitAndQuotaCheck{baseMid},
		&ValidateJSON{BaseMiddleware: baseMid},
		&TransformHeaders{baseMid},
	)...).Then(proxyHandler)
	return chain
}
