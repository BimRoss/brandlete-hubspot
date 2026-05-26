# brandlete-hubspot

HubSpot integration service for [Brandlete](https://brandlete.com). One Go binary, two surfaces:

1. **Inbound webhooks** (`POST /webhook/*`) — receive events from the Brandlete app/lander and write them into HubSpot (contacts, deals, notes).
2. **MCP server** (`POST /mcp`) — Streamable HTTP MCP endpoint pre-wired with Brandlete's HubSpot portal, so any MCP client (Claude Desktop, Cursor, claude-code, server-to-server) can read and write CRM data.

Single tenant (Brandlete's HubSpot) for v0. Multi-tenant later.

---

## Quick start

### 1. Create the HubSpot private app

In Brandlete's HubSpot portal:

1. Go to **Settings → Integrations → Private Apps**.
2. Click **Create private app**, name it (e.g. `brandlete-hubspot-service`).
3. Under **Scopes**, grant:
   - `crm.objects.contacts.read` + `crm.objects.contacts.write`
   - `crm.objects.deals.read` + `crm.objects.deals.write`
   - `crm.objects.companies.read`
   - `crm.objects.notes.write`
   - `crm.schemas.deals.read`
4. Create the app and **copy the access token**. This is the `HUBSPOT_TOKEN` below.

> Token starts with `pat-na1-...` and is shown once. Store it in your secret manager immediately.

### 2. Generate two secrets for the service itself

```bash
# For inbound webhooks (Brandlete app → this service)
openssl rand -hex 32   # → WEBHOOK_SECRET

# For MCP clients (Claude Desktop / etc. → this service)
openssl rand -hex 32   # → MCP_BEARER_TOKEN
```

### 3. Run it

```bash
docker run --rm -p 8080:8080 \
  -e HUBSPOT_TOKEN=pat-na1-... \
  -e WEBHOOK_SECRET=$WEBHOOK_SECRET \
  -e MCP_BEARER_TOKEN=$MCP_BEARER_TOKEN \
  ghcr.io/bimross/brandlete-hubspot:latest
```

Or from source:

```bash
go run ./cmd/server
```

For local development without a real HubSpot portal, set `DRY_RUN=true` — all HubSpot writes get logged instead of executed, and `HUBSPOT_TOKEN` becomes optional.

---

## Configuration

| Env var            | Required | Default | Purpose                                                                       |
| ------------------ | -------- | ------- | ----------------------------------------------------------------------------- |
| `HUBSPOT_TOKEN`    | yes\*    | —       | Brandlete HubSpot private app access token. \*Optional when `DRY_RUN=true`.   |
| `WEBHOOK_SECRET`   | yes      | —       | Shared secret the Brandlete app sends as `X-Brandlete-Webhook-Secret`.        |
| `MCP_BEARER_TOKEN` | yes      | —       | Bearer token MCP clients send as `Authorization: Bearer …`.                   |
| `PORT`             | no       | `8080`  | Listen port.                                                                  |
| `DRY_RUN`          | no       | `false` | When `true`, log would-be HubSpot calls instead of executing. Useful for dev. |

---

## Endpoints

### Webhooks

#### `POST /webhook/demo-request`

Coach / parent / director requested a Brandlete demo. Upserts a contact, attaches a note describing the request.

```bash
curl -X POST https://hubspot.brandlete.makeacompany.ai/webhook/demo-request \
  -H "X-Brandlete-Webhook-Secret: $WEBHOOK_SECRET" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Jane Coach",
    "email": "jane@example.com",
    "company": "Example HS Lacrosse",
    "phone": "+1-555-0100",
    "role": "Head Coach",
    "source": "brandlete.com/demo",
    "notes": "Interested in fall onboarding for 25 athletes."
  }'
```

Response: `{"contact_id":"123456"}` on success.

#### `POST /webhook/contact-created`

Generic contact-form submission. Upserts a contact, attaches a note with the message.

```bash
curl -X POST https://hubspot.brandlete.makeacompany.ai/webhook/contact-created \
  -H "X-Brandlete-Webhook-Secret: $WEBHOOK_SECRET" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Pat Parent",
    "email": "pat@example.com",
    "message": "Question about pricing.",
    "source": "brandlete.com/contact"
  }'
```

### MCP

#### `POST /mcp`

JSON-RPC 2.0 MCP endpoint. Authenticate with `Authorization: Bearer $MCP_BEARER_TOKEN`.

**Available tools (v0):**

- `whoami` — returns the HubSpot account this server is authed against. Smoke test.
- `search-contacts` — search by name / email / company. (Stub in v0.)

Wire into Claude Desktop:

```json
{
  "mcpServers": {
    "brandlete-hubspot": {
      "url": "https://hubspot.brandlete.makeacompany.ai/mcp",
      "headers": {
        "Authorization": "Bearer <your-MCP_BEARER_TOKEN>"
      }
    }
  }
}
```

**Health:** `GET /healthz` and `GET /readyz` both return 200 when the server is up.

---

## Building & releasing

CI builds and publishes container images on every push to `main` and every `v*` tag:

- `ghcr.io/bimross/brandlete-hubspot:latest` (main branch)
- `ghcr.io/bimross/brandlete-hubspot:v0.1.0` (tags)
- `ghcr.io/bimross/brandlete-hubspot:sha-<commit>` (every commit)

Images are multi-arch (linux/amd64, linux/arm64), built on distroless for ~15MB.

---

## Roadmap

- [x] v0 scaffold: webhooks (demo-request, contact-created), MCP (whoami, search-contacts).
- [ ] Wire real Brandlete HubSpot private app token.
- [ ] Confirm Brandlete's deal pipeline + "Demo Requested" stage ID; add deal creation to demo-request webhook.
- [ ] Implement remaining MCP tools: `search-deals`, `get-contact-activity`, `add-note`.
- [ ] Deploy to MaC k8s.
- [ ] Multi-tenant: per-customer HubSpot tokens, scoped MCP auth.

## License

MIT — matches `@hubspot/mcp-server`'s license.
