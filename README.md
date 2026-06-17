Option A: The Unified Endpoint (Recommended)
The unified endpoint POST /api/v1/events dynamically parses, validates, and routes any event type based on the "event_name" field.

1. Ingest a Login Event
bash
curl -X POST http://localhost:8080/api/v1/events \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer dev-secure-token-12345" \
  -d '{
    "event_name": "user.login",
    "source": "curl-test",
    "user_id": "user_98765",
    "method": "LOGIN_METHOD_EMAIL",
    "success": true,
    "ip_address": "127.0.0.1",
    "user_agent": "curl/8.0.0"
  }'
2. Ingest a Level Progression Event
bash
curl -X POST http://localhost:8080/api/v1/events \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer dev-secure-token-12345" \
  -d '{
    "event_name": "level.progression",
    "source": "curl-test",
    "user_id": "user_98765",
    "level_id": "stage_1_boss",
    "action": "LEVEL_ACTION_COMPLETED",
    "score": 5200,
    "duration_seconds": 120
  }'
3. Ingest a Transaction Event
bash
curl -X POST http://localhost:8080/api/v1/events \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer dev-secure-token-12345" \
  -d '{
    "event_name": "commerce.purchase",
    "source": "curl-test",
    "user_id": "user_98765",
    "transaction_id": "tx_abc123",
    "amount": 24.99,
    "currency": "USD",
    "items": [
      {
        "item_id": "item_1",
        "name": "Power-up Pack",
        "price": 24.99,
        "quantity": 1
      }
    ],
    "status": "TRANSACTION_STATUS_SUCCESS"
  }'