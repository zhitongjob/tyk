package main

import (
	"errors"
	"net/http"

	"github.com/TykTechnologies/tyk/apidef"
	"plugin"
)

// TransformMiddleware is a middleware that will apply a template to a request body to transform it's contents ready for an upstream API
type MWGoPlugin struct {
	*BaseMiddleware
	processRequest func (http.ResponseWriter, *http.Request, *apidef.APIDefinition) (error, int)
}

func (t *MWGoPlugin) Name() string {
	return "MWGoPlugin"
}

func (t *MWGoPlugin) loadPlugin(path string, name string) error {
	p, err := plugin.Open(path)
	if err != nil {
		return err
	}

	s, err := p.Lookup(name)
	if err != nil {
		return err
	}

	pr, ok := s.(func (http.ResponseWriter, *http.Request, *apidef.APIDefinition) (error, int))
	if !ok {
		return errors.New("Function signature incorrect")
	}

	t.processRequest = pr
	return nil
}

func (t *MWGoPlugin) IsEnabledForSpec() bool {
	if t.Spec.CustomMiddleware.Driver == apidef.GoDriver {
		if len(t.Spec.CustomMiddleware.Pre) == 1 {
			if t.Spec.CustomMiddleware.Pre[0].Path != "" && t.Spec.CustomMiddleware.Pre[0].Name != ""{
				if err := t.loadPlugin(t.Spec.CustomMiddleware.Pre[0].Path, t.Spec.CustomMiddleware.Pre[0].Name); err != nil {
					log.Errorf("Failed to load plugin: %v, error: %v", t.Spec.CustomMiddleware.Pre[0].Path, err)
					return false
				}
				return true
			}
		}
	}
	return false
}

// ProcessRequest will run any checks on the request on the way through the system, return an error to have the chain fail
func (t *MWGoPlugin) ProcessRequest(w http.ResponseWriter, r *http.Request, _ interface{}) (error, int) {
	if t.processRequest != nil {
		return t.processRequest(w, r, t.Spec.APIDefinition)
	}
	return nil, 200
}
