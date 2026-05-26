package webhook

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/BimRoss/brandlete-hubspot/internal/hubspot"
)

func Register(mux *http.ServeMux, hs *hubspot.Client, secret string, log *slog.Logger) {
	h := &handler{hs: hs, secret: secret, log: log}
	mux.HandleFunc("POST /webhook/demo-request", h.authMiddleware(h.demoRequest))
	mux.HandleFunc("POST /webhook/contact-created", h.authMiddleware(h.contactCreated))
}

type handler struct {
	hs     *hubspot.Client
	secret string
	log    *slog.Logger
}

func (h *handler) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		got := r.Header.Get("X-Brandlete-Webhook-Secret")
		if subtle.ConstantTimeCompare([]byte(got), []byte(h.secret)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

type DemoRequest struct {
	Name    string `json:"name"`
	Email   string `json:"email"`
	Company string `json:"company,omitempty"`
	Phone   string `json:"phone,omitempty"`
	Role    string `json:"role,omitempty"`
	Source  string `json:"source"`
	Notes   string `json:"notes,omitempty"`
}

func (h *handler) demoRequest(w http.ResponseWriter, r *http.Request) {
	var req DemoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := req.validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	first, last := splitName(req.Name)
	props := map[string]string{"firstname": first, "lastname": last}
	if req.Company != "" {
		props["company"] = req.Company
	}
	if req.Phone != "" {
		props["phone"] = req.Phone
	}
	if req.Role != "" {
		props["jobtitle"] = req.Role
	}

	contactID, err := h.hs.UpsertContact(r.Context(), req.Email, props)
	if err != nil {
		h.log.Error("upsert contact failed", "err", err, "email", req.Email)
		http.Error(w, "hubspot error", http.StatusBadGateway)
		return
	}

	if req.Notes != "" || req.Source != "" {
		noteBody := "Demo request via " + req.Source
		if req.Notes != "" {
			noteBody += "\n\n" + req.Notes
		}
		if _, err := h.hs.CreateNote(r.Context(), noteBody, contactID); err != nil {
			h.log.Error("create note failed", "err", err, "contact_id", contactID)
			// Don't fail the webhook on note failure — contact was upserted.
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"contact_id": contactID})
}

type ContactCreated struct {
	Name    string `json:"name"`
	Email   string `json:"email"`
	Message string `json:"message,omitempty"`
	Source  string `json:"source"`
}

func (h *handler) contactCreated(w http.ResponseWriter, r *http.Request) {
	var req ContactCreated
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Email == "" {
		http.Error(w, "email required", http.StatusBadRequest)
		return
	}

	first, last := splitName(req.Name)
	props := map[string]string{"firstname": first, "lastname": last}

	contactID, err := h.hs.UpsertContact(r.Context(), req.Email, props)
	if err != nil {
		h.log.Error("upsert contact failed", "err", err, "email", req.Email)
		http.Error(w, "hubspot error", http.StatusBadGateway)
		return
	}

	if req.Message != "" {
		noteBody := "Contact form (" + req.Source + "):\n\n" + req.Message
		if _, err := h.hs.CreateNote(r.Context(), noteBody, contactID); err != nil {
			h.log.Error("create note failed", "err", err, "contact_id", contactID)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"contact_id": contactID})
}

func (d DemoRequest) validate() error {
	if d.Email == "" {
		return errors.New("email required")
	}
	if d.Source == "" {
		return errors.New("source required")
	}
	return nil
}

func splitName(name string) (first, last string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", ""
	}
	parts := strings.SplitN(name, " ", 2)
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], parts[1]
}
