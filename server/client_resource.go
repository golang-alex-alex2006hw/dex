package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path"

	"github.com/coreos/dex/client/manager"
	phttp "github.com/coreos/dex/pkg/http"
	"github.com/coreos/dex/pkg/log"
	schema "github.com/coreos/dex/schema/workerschema"
)

type clientResource struct {
	manager *manager.ClientManager
}

func registerClientResource(prefix string, manager *manager.ClientManager) (string, http.Handler) {
	mux := http.NewServeMux()
	c := &clientResource{
		manager: manager,
	}
	relPath := "clients"
	absPath := path.Join(prefix, relPath)
	mux.Handle(absPath, c)
	return relPath, mux
}

func (c *clientResource) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		c.list(w, r)
	case "POST":
		c.create(w, r)
	default:
		msg := fmt.Sprintf("HTTP %s method not supported for this resource", r.Method)
		writeAPIError(w, http.StatusMethodNotAllowed, newAPIError(errorInvalidRequest, msg))
	}
}

func (c *clientResource) list(w http.ResponseWriter, r *http.Request) {
	cs, err := c.manager.All()
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, newAPIError(errorServerError, "error listing clients"))
		return
	}

	scs := make([]*schema.Client, len(cs))
	for i, ci := range cs {
		sc := schema.MapClientToSchemaClient(ci)
		scs[i] = &sc
	}

	page := schema.ClientPage{
		Clients: scs,
	}
	writeResponseWithBody(w, http.StatusOK, page)
}

func (c *clientResource) create(w http.ResponseWriter, r *http.Request) {
	ct := r.Header.Get("content-type")
	if ct != "application/json" {
		log.Debugf("Unsupported request content-type: %v", ct)
		writeAPIError(w, http.StatusBadRequest, newAPIError(errorInvalidRequest, "unsupported content-type"))
		return
	}

	var sc schema.Client
	dec := json.NewDecoder(r.Body)
	err := dec.Decode(&sc)
	if err != nil {
		log.Debugf("Error decoding request body: %v", err)
		writeAPIError(w, http.StatusBadRequest, newAPIError(errorInvalidRequest, "unable to decode request body"))
		return
	}

	ci, err := schema.MapSchemaClientToClient(sc)
	if err != nil {
		log.Debugf("Invalid request data: %v", err)
		writeAPIError(w, http.StatusBadRequest, newAPIError(errorInvalidClientMetadata, "missing or invalid field: redirectURIs"))
		return
	}

	if err := ci.Metadata.Valid(); err != nil {
		log.Debugf("ClientMetadata invalid: %v", err)
		writeAPIError(w, http.StatusBadRequest, newAPIError(errorInvalidClientMetadata, err.Error()))
		return
	}
	creds, err := c.manager.New(ci)

	if err != nil {
		log.Errorf("Failed creating client: %v", err)
		writeAPIError(w, http.StatusInternalServerError, newAPIError(errorServerError, "unable to create client"))
		return
	}
	ci.Credentials = *creds

	ssc := schema.MapClientToSchemaClientWithSecret(ci)
	w.Header().Add("Location", phttp.NewResourceLocation(r.URL, ci.Credentials.ID))
	writeResponseWithBody(w, http.StatusCreated, ssc)
}
