// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: BUSL-1.1

package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/hashicorp/go-hclog"

	"github.com/hashicorp/consul/internal/resource"
	"github.com/hashicorp/consul/proto-public/pbresource"
)

const (
	HeaderConsulToken     = "x-consul-token"
	HeaderConsistencyMode = "x-consul-consistency-mode"
)

func NewHandler(
	client pbresource.ResourceServiceClient,
	registry resource.Registry,
	parseToken func(req *http.Request, token *string),
	logger hclog.Logger) http.Handler {
	mux := http.NewServeMux()
	for _, t := range registry.Types() {
		// List Endpoint
		base := strings.ToLower(fmt.Sprintf("/%s/%s/%s", t.Type.Group, t.Type.GroupVersion, t.Type.Kind))
		mux.Handle(base, http.StripPrefix(base, &listHandler{t, client, parseToken, logger}))

		// Individual Resource Endpoints
		prefix := strings.ToLower(fmt.Sprintf("%s/", base))
		logger.Info("Registered resource endpoint", "endpoint", prefix)
		mux.Handle(prefix, http.StripPrefix(prefix, &resourceHandler{t, client, parseToken, logger}))
	}

	return mux
}

type writeRequest struct {
	Metadata map[string]string `json:"metadata"`
	Data     json.RawMessage   `json:"data"`
	Owner    *pbresource.ID    `json:"owner"`
}

type resourceHandler struct {
	reg        resource.Registration
	client     pbresource.ResourceServiceClient
	parseToken func(req *http.Request, token *string)
	logger     hclog.Logger
}

func (h *resourceHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var token string
	h.parseToken(r, &token)
	ctx := metadata.AppendToOutgoingContext(r.Context(), HeaderConsulToken, token)
	switch r.Method {
	case http.MethodPut:
		h.handleWrite(w, r, ctx)
	case http.MethodDelete:
		h.handleDelete(w, r, ctx)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
}

func (h *resourceHandler) handleWrite(w http.ResponseWriter, r *http.Request, ctx context.Context) {
	var req writeRequest
	// convert req body to writeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Request body didn't follow schema."))
	}
	// convert data struct to proto message
	data := h.reg.Proto.ProtoReflect().New().Interface()
	if err := protojson.Unmarshal(req.Data, data); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Request body didn't follow schema."))
	}
	// proto message to any
	anyProtoMsg, err := anypb.New(data)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		h.logger.Error("Failed to convert proto message to any type", "error", err)
		return
	}

	tenancyInfo, queryParams := parseParams(r)

	rsp, err := h.client.Write(ctx, &pbresource.WriteRequest{
		Resource: &pbresource.Resource{
			Id: &pbresource.ID{
				Type:    h.reg.Type,
				Tenancy: tenancyInfo,
				Name:    queryParams["resourceName"],
			},
			Owner:    req.Owner,
			Version:  queryParams["version"],
			Metadata: req.Metadata,
			Data:     anyProtoMsg,
		},
	})
	if err != nil {
		handleResponseError(err, w, h.logger)
		return
	}

	output, err := jsonMarshal(rsp.Resource)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		h.logger.Error("Failed to unmarshal GRPC resource response", "error", err)
		return
	}
	w.Write(output)
}

func parseParams(r *http.Request) (tenancy *pbresource.Tenancy, queryParams map[string]string) {
	params := r.URL.Query()
	tenancy = &pbresource.Tenancy{
		Partition: params.Get("partition"),
		PeerName:  params.Get("peer_name"),
		Namespace: params.Get("namespace"),
	}
	resourceName := path.Base(r.URL.Path)
	if resourceName == "." || resourceName == "/" {
		resourceName = ""
	}

	queryParams = make(map[string]string)
	queryParams["resourceName"] = resourceName
	queryParams["version"] = params.Get("version")
	queryParams["namePrefix"] = params.Get("name_prefix")

	if params.Has("consistent") {
		queryParams["consistent"] = "true"
	}

	return tenancy, queryParams
}

func jsonMarshal(res *pbresource.Resource) ([]byte, error) {
	output, err := protojson.Marshal(res)
	if err != nil {
		return nil, err
	}

	var stuff map[string]any
	if err := json.Unmarshal(output, &stuff); err != nil {
		return nil, err
	}

	delete(stuff["data"].(map[string]any), "@type")
	return json.MarshalIndent(stuff, "", "  ")
}

func handleResponseError(err error, w http.ResponseWriter, logger hclog.Logger) {
	if e, ok := status.FromError(err); ok {
		switch e.Code() {
		case codes.InvalidArgument:
			w.WriteHeader(http.StatusBadRequest)
			logger.Info("User has mal-formed request", "error", err)
		case codes.NotFound:
			w.WriteHeader(http.StatusNotFound)
			logger.Info("Received error from resource service: Not found", "error", err)
		case codes.PermissionDenied:
			w.WriteHeader(http.StatusForbidden)
			logger.Info("Received error from resource service: User not authenticated", "error", err)
		case codes.Aborted:
			w.WriteHeader(http.StatusConflict)
			logger.Info("Received error from resource service: the request conflict with the current state of the target resource", "error", err)
		default:
			w.WriteHeader(http.StatusInternalServerError)
			logger.Error("Received error from resource service", "error", err)
		}
	} else {
		w.WriteHeader(http.StatusInternalServerError)
		logger.Error("Received error from resource service: not able to parse error returned", "error", err)
	}
	w.Write([]byte(err.Error()))
}

// Note: The HTTP endpoints do not accept UID since it is quite unlikely that the user will have access to it
func (h *resourceHandler) handleDelete(w http.ResponseWriter, r *http.Request, ctx context.Context) {
	tenancyInfo, queryParams := parseParams(r)
	_, err := h.client.Delete(ctx, &pbresource.DeleteRequest{
		Id: &pbresource.ID{
			Type:    h.reg.Type,
			Tenancy: tenancyInfo,
			Name:    queryParams["resourceName"],
		},
		Version: queryParams["version"],
	})
	if err != nil {
		handleResponseError(err, w, h.logger)
		return
	}
	w.WriteHeader(http.StatusNoContent)
	w.Write([]byte("{}"))
}

type listHandler struct {
	reg        resource.Registration
	client     pbresource.ResourceServiceClient
	parseToken func(req *http.Request, token *string)
	logger     hclog.Logger
}

func (h *listHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var token string
	h.parseToken(r, &token)
	ctx := metadata.AppendToOutgoingContext(r.Context(), HeaderConsulToken, token)

	tenancyInfo, queryParams := parseParams(r)
	if queryParams["consistent"] == "true" {
		ctx = metadata.AppendToOutgoingContext(ctx, HeaderConsistencyMode, "consistent")
	}

	rsp, err := h.client.List(ctx, &pbresource.ListRequest{
		Type:       h.reg.Type,
		Tenancy:    tenancyInfo,
		NamePrefix: queryParams["namePrefix"],
	})
	if err != nil {
		handleResponseError(err, w, h.logger)
		return
	}

	output := make([]json.RawMessage, len(rsp.Resources))
	for idx, res := range rsp.Resources {
		b, err := jsonMarshal(res)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			h.logger.Error("Failed to unmarshal GRPC resource response", "error", err)
			return
		}
		output[idx] = b
	}

	b, err := json.MarshalIndent(struct {
		Resources []json.RawMessage `json:"resources"`
	}{output}, "", "  ")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		h.logger.Error("Failed to correcty format the list response", "error", err)
		return
	}
	w.Write(b)
}
