package hubspot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

const baseURL = "https://api.hubapi.com"

type Client struct {
	token  string
	dryRun bool
	http   *http.Client
	log    *slog.Logger
}

func New(token string, dryRun bool, log *slog.Logger) *Client {
	return &Client{
		token:  token,
		dryRun: dryRun,
		http:   &http.Client{Timeout: 15 * time.Second},
		log:    log,
	}
}

type Contact struct {
	ID         string            `json:"id,omitempty"`
	Properties map[string]string `json:"properties"`
}

// UpsertContact creates a contact, or updates it if the email already exists.
// HubSpot's upsert pattern: try create, on 409 with existing id, update.
func (c *Client) UpsertContact(ctx context.Context, email string, props map[string]string) (string, error) {
	if props == nil {
		props = map[string]string{}
	}
	props["email"] = email

	if c.dryRun {
		c.log.Info("dry-run upsert contact", "email", email, "props", props)
		return "dry-run-contact-id", nil
	}

	body := map[string]any{"properties": props}
	var created struct {
		ID string `json:"id"`
	}
	resp, err := c.do(ctx, http.MethodPost, "/crm/v3/objects/contacts", body, &created)
	if err == nil {
		return created.ID, nil
	}

	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.Status == http.StatusConflict {
		// Existing contact — extract id from the error body and PATCH.
		if apiErr.ExistingID != "" {
			patch := map[string]any{"properties": props}
			path := fmt.Sprintf("/crm/v3/objects/contacts/%s", apiErr.ExistingID)
			if _, err := c.do(ctx, http.MethodPatch, path, patch, nil); err != nil {
				return "", err
			}
			return apiErr.ExistingID, nil
		}
	}
	_ = resp
	return "", err
}

// CreateNote attaches a note to an object (contact, deal, etc.).
func (c *Client) CreateNote(ctx context.Context, body string, associatedContactID string) (string, error) {
	if c.dryRun {
		c.log.Info("dry-run create note", "body_preview", preview(body, 80), "contact_id", associatedContactID)
		return "dry-run-note-id", nil
	}

	payload := map[string]any{
		"properties": map[string]any{
			"hs_note_body":   body,
			"hs_timestamp":   time.Now().UTC().Format(time.RFC3339),
		},
		"associations": []map[string]any{
			{
				"to":    map[string]string{"id": associatedContactID},
				"types": []map[string]any{{"associationCategory": "HUBSPOT_DEFINED", "associationTypeId": 202}}, // note → contact
			},
		},
	}

	var created struct {
		ID string `json:"id"`
	}
	if _, err := c.do(ctx, http.MethodPost, "/crm/v3/objects/notes", payload, &created); err != nil {
		return "", err
	}
	return created.ID, nil
}

// WhoAmI hits the account-info endpoint as a smoke test of token validity.
func (c *Client) WhoAmI(ctx context.Context) (map[string]any, error) {
	if c.dryRun {
		return map[string]any{"dry_run": true}, nil
	}
	var out map[string]any
	if _, err := c.do(ctx, http.MethodGet, "/account-info/v3/details", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// SearchContacts is a TODO stub — real impl uses /crm/v3/objects/contacts/search.
func (c *Client) SearchContacts(ctx context.Context, query string) ([]map[string]any, error) {
	return nil, errors.New("not implemented yet")
}

type APIError struct {
	Status     int
	Body       string
	ExistingID string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("hubspot api error: status=%d body=%s", e.Status, e.Body)
}

func (c *Client) do(ctx context.Context, method, path string, in any, out any) (*http.Response, error) {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		apiErr := &APIError{Status: resp.StatusCode, Body: string(respBody)}
		// HubSpot returns existing id in error body for some 409s. Parse if present.
		var parsed struct {
			Message string `json:"message"`
			Errors  []struct {
				ID string `json:"id"`
			} `json:"errors"`
		}
		_ = json.Unmarshal(respBody, &parsed)
		if len(parsed.Errors) > 0 {
			apiErr.ExistingID = parsed.Errors[0].ID
		}
		return resp, apiErr
	}

	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return resp, err
		}
	}
	return resp, nil
}

func preview(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
