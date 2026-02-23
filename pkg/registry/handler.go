// Package registry implements the Splice Registry API for external wallet integration.
//
// It exposes contract discovery endpoints that allow wallets like Canton Loop
// to obtain TransferFactory contract IDs and CreatedEventBlobs for explicit
// contract disclosure during Splice-standard token transfers.
package registry

import (
	"encoding/base64"
	"encoding/json"
	"net/http"

	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/token"

	"go.uber.org/zap"
)

// Handler serves the Splice Registry API endpoints.
type Handler struct {
	tokenClient token.Token
	logger      *zap.Logger
}

// NewHandler creates a registry handler.
func NewHandler(tokenClient token.Token, logger *zap.Logger) *Handler {
	return &Handler{
		tokenClient: tokenClient,
		logger:      logger,
	}
}

// TransferFactoryResponse is the JSON body returned by the transfer-factory endpoint.
type TransferFactoryResponse struct {
	ContractID       string             `json:"contract_id"`
	CreatedEventBlob string             `json:"created_event_blob"`
	TemplateID       templateIDResponse `json:"template_id"`
}

type templateIDResponse struct {
	PackageID  string `json:"package_id"`
	ModuleName string `json:"module_name"`
	EntityName string `json:"entity_name"`
}

type errorResponse struct {
	Error string `json:"error"`
}

// ServeHTTP handles POST requests and returns the active TransferFactory contract.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	info, err := h.tokenClient.GetTransferFactory(r.Context())
	if err != nil {
		h.logger.Error("Failed to get TransferFactory", zap.Error(err))
		h.writeError(w, http.StatusInternalServerError, "failed to retrieve transfer factory")
		return
	}

	if len(info.CreatedEventBlob) == 0 {
		h.logger.Warn("TransferFactory found but CreatedEventBlob is empty",
			zap.String("contract_id", info.ContractID))
	}

	h.writeJSON(w, http.StatusOK, TransferFactoryResponse{
		ContractID:       info.ContractID,
		CreatedEventBlob: base64.StdEncoding.EncodeToString(info.CreatedEventBlob),
		TemplateID: templateIDResponse{
			PackageID:  info.TemplateID.PackageID,
			ModuleName: info.TemplateID.ModuleName,
			EntityName: info.TemplateID.EntityName,
		},
	})
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Warn("failed to write JSON response", zap.Error(err))
	}
}

func (h *Handler) writeError(w http.ResponseWriter, status int, message string) {
	h.writeJSON(w, status, errorResponse{Error: message})
}
