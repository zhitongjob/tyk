package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/xeipuuv/gojsonschema"

	"github.com/TykTechnologies/tyk/apidef"
)

type ValidateJSON struct {
	BaseMiddleware
}

func (k *ValidateJSON) Name() string {
	return "ValidateJSON"
}

func (k *ValidateJSON) EnabledForSpec() bool {
	for _, v := range k.Spec.VersionData.Versions {
		if len(v.ExtendedPaths.ValidateJSON) > 0 {
			return true
		}
	}

	return false
}

// ProcessRequest will run any checks on the request on the way through the system, return an error to have the chain fail
func (k *ValidateJSON) ProcessRequest(w http.ResponseWriter, r *http.Request, _ interface{}) (error, int) {

	_, versionPaths, _, _ := k.Spec.Version(r)
	found, meta := k.Spec.CheckSpecMatchesStatus(r, versionPaths, ValidateJSONRequest)
	if !found {
		return nil, 200
	}

	mmeta := meta.(*apidef.ValidatePathMeta)
	if mmeta.ValidateWith64 == "" {
		return errors.New("no schemas to validate against"), http.StatusInternalServerError
	}

	rCopy := copyRequest(r)
	bodyBytes, err := ioutil.ReadAll(rCopy.Body)
	if err != nil {
		return err, http.StatusInternalServerError
	}
	defer rCopy.Body.Close()

	schemaBytes, err := base64.StdEncoding.DecodeString(mmeta.ValidateWith64)
	if err != nil {
		return errors.New("unable to base64 decode schema"), http.StatusInternalServerError
	}

	result, err := k.validate(bodyBytes, schemaBytes)
	if err != nil {
		return err, http.StatusUnprocessableEntity
	}

	if !result.Valid() {
		errStr := "payload validation failed"
		for _, desc := range result.Errors() {
			errStr = fmt.Sprintf("%s, %s", errStr, desc)
		}

		if mmeta.ValidationErrorResponseCode == 0 {
			mmeta.ValidationErrorResponseCode = http.StatusUnprocessableEntity
		}

		return errors.New(errStr), mmeta.ValidationErrorResponseCode
	}

	return nil, http.StatusOK
}

func (k *ValidateJSON) validate(input []byte, rules []byte) (*gojsonschema.Result, error) {
	inputLoader := gojsonschema.NewBytesLoader(input)
	rulesLoader := gojsonschema.NewBytesLoader(rules)

	return gojsonschema.Validate(rulesLoader, inputLoader)
}
