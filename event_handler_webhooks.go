package main

import (
	"html/template"
	"net/http"
	"strings"
	"net/url"
	"bytes"
	"crypto/md5"
	"io"
	"io/ioutil"
	"encoding/hex"
	"encoding/json"
	"github.com/lonelycode/tykcommon"
)

type WebHookRequestMethod string
const (
	WH_GET WebHookRequestMethod = "GET"
	WH_PUT WebHookRequestMethod = "PUT"
	WH_POST WebHookRequestMethod = "POST"
	WH_DELETE WebHookRequestMethod = "DELETE"
	WH_PATCH WebHookRequestMethod = "PATCH"

	// Define the Event Handler name so we can register it
	EH_WebHook	tykcommon.TykEventHandlerName = "eh_web_hook_handler"
)

type WebHookHandlerConf struct {
	Method string	`bson:"method" json:"method"`
	TargetPath string	`bson:"target_path" json:"target_path"`
	TemplatePath string	`bson:"template_path" json:"template_path"`
	HeaderList map[string]string	`bson:"header_map" json:"header_map"`
	EventTimeout int64	`bson:"event_timeout" json:"event_timeout"`
}

// WebHookHandler is an event handler that triggers web hooks
type WebHookHandler struct {
	conf WebHookHandlerConf
	template *template.Template
	store *RedisStorageManager
}

// Not Pretty, but will avoi dmillions of connections
var WebHookRedisStorePointer *RedisStorageManager

// GetRedisInterfacePointer creates a reference to a redis connection pool that can be shared across all webhook instances
func GetRedisInterfacePointer() *RedisStorageManager {
	if WebHookRedisStorePointer == nil {
		WebHookRedisStorePointer = &RedisStorageManager{KeyPrefix: "webhook.cache."}
		WebHookRedisStorePointer.Connect()
	}

	return WebHookRedisStorePointer
}

// createConfigObject by default tyk will provide a ma[string]interface{} type as a conf, converting it
// specifically here makes it easier to handle, only happens once, so not a massive issue, but not pretty
func (w WebHookHandler) createConfigObject(handlerConf interface{}) (WebHookHandlerConf, error) {
	newConf := WebHookHandlerConf{}

	asJSON, _ := json.Marshal(handlerConf)
	if err := json.Unmarshal(asJSON, &newConf); err != nil {
		log.Error("Format of webhook configuration is incorrect: ", err)
		return newConf, err
	}

	return newConf, nil
}

// New enables the init of event handler instances when they are created on ApiSpec creation
func (w WebHookHandler) New(handlerConf interface{}) (TykEventHandler, error) {
	thisHandler := WebHookHandler{}
	var confErr error
	thisHandler.conf, confErr = w.createConfigObject(handlerConf)

	if confErr != nil {
		log.Error("Problem getting configuration, skipping. ", confErr)
		return thisHandler, confErr
	}

	// Get a storage reference
	thisHandler.store = GetRedisInterfacePointer()

	// Pre-load template on init
	webHookTemplate, tErr := template.ParseFiles(thisHandler.conf.TemplatePath)
	if tErr != nil {
		log.Error("Failed to load webhook template! Using defult. Error was: ", tErr)
		webHookTemplate, _ = template.ParseFiles("templates/default_webhook.json")
	}
	thisHandler.template = webHookTemplate

	if !thisHandler.checkURL(thisHandler.conf.TargetPath) {
		log.Error("Init failed for this webhook, invalid URL, URL must be absolute")
	}


	return thisHandler, nil
}

// hookFired checks if an event has been fired within the EventTimeout setting
func (w WebHookHandler) WasHookFired(checksum string) bool {
	_, keyErr := w.store.GetKey(checksum)
	if keyErr != nil {
		// Key not found, so hook is in limit
		log.Info("Event can fire, no duplicates found")
		return false
	}

	return true
}

// setHookFired will create an expiring key for the checksum of the event
func (w WebHookHandler) setHookFired(checksum string) {
	log.Warning("Setting Webhook Checksum: ", checksum)
	w.store.SetKey(checksum, "1", w.conf.EventTimeout)
}

func (w WebHookHandler) getRequestMethod(m string) WebHookRequestMethod {
	switch strings.ToUpper(m) {
		case "GET": return WH_GET
		case "PUT": return WH_PUT
		case "POST": return WH_POST
		case "DELETE": return WH_DELETE
		case "PATCH": return WH_DELETE
		default: log.Warning("Method must be one of GET, PUT, POST, DELETE or PATCH, defaulting to GET"); return WH_GET
	}
}

func (w WebHookHandler) checkURL(r string) bool {
	log.Debug("Checking URL: ", r)
	_, urlErr := url.ParseRequestURI(r)
	if urlErr != nil {
		log.Error("Failed to parse URL! ", urlErr, r)
		return false
	}
	return true
}

func (w WebHookHandler) GetChecksum(reqBody string) (string, error) {
	var rawRequest bytes.Buffer
	// We do this twice because fuck it.
	localRequest, _ := http.NewRequest(string(w.getRequestMethod(w.conf.Method)), w.conf.TargetPath, bytes.NewBuffer([]byte(reqBody)))

	localRequest.Write(&rawRequest)
	h := md5.New()
	io.WriteString(h, rawRequest.String())

	reqChecksum := hex.EncodeToString(h.Sum(nil))

	return reqChecksum, nil
}

func (w WebHookHandler) BuildRequest(reqBody string) (*http.Request, error) {
	req, reqErr := http.NewRequest(string(w.getRequestMethod(w.conf.Method)), w.conf.TargetPath, bytes.NewBuffer([]byte(reqBody)))
	if reqErr != nil {
		log.Error("Failed to create request object: ", reqErr)
		return nil, reqErr
	}

	req.Header.Add("User-Agent", "Tyk-Hookshot")

	for key, val := range (w.conf.HeaderList) {
		req.Header.Add(key, val)
	}

	return req, nil
}

func (w WebHookHandler) CreateBody(em EventMessage) (string, error) {
	var reqBody bytes.Buffer
	w.template.Execute(&reqBody, em)

	return reqBody.String(), nil
}

// HandleEvent will be fired when the event handler instance is found in an APISpec EventPaths object during a request chain
func (w WebHookHandler) HandleEvent(em EventMessage) {

	// Inject event message into template, render to string
	reqBody, _ := w.CreateBody(em)

	// Construct request (method, body, params)
	req, reqErr := w.BuildRequest(reqBody)
	if reqErr != nil {
		return
	}

	// Generate signature for request
	reqChecksum, _ := w.GetChecksum(reqBody)

	// Check request velocity for this hook (wasHookFired())
	if !w.WasHookFired(reqChecksum) {
		// Fire web hook routine (setHookFired())

		client := &http.Client{}
		resp, doReqErr := client.Do(req)

		if doReqErr != nil {
			log.Error("Webhook request failed: ", doReqErr)
		} else {
			defer resp.Body.Close()
			content, readErr := ioutil.ReadAll(resp.Body)
			if readErr == nil {
				log.Warning(string(content))
			} else {
				log.Error(readErr)
			}
		}

		w.setHookFired(reqChecksum)
	}
}