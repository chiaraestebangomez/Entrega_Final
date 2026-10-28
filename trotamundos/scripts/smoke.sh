#!/usr/bin/env bash
set -euo pipefail

BASE="${BASE_URL:-http://localhost:8080}"
MOCK_KEY="${MOCK_CONTROL_KEY:-dev-mock-key}"
TS="$(date +%s)${RANDOM}"
TRAVELER="smoke+${TS}@example.com"
GUEST="guest+${TS}@example.com"
PASSWORD="secreto12345"
START="$(date -u -d '+45 days' +%Y-%m-%dT08:00:00Z 2>/dev/null || date -u -v+45d +%Y-%m-%dT08:00:00Z)"
END="$(date -u -d '+52 days' +%Y-%m-%dT18:00:00Z 2>/dev/null || date -u -v+52d +%Y-%m-%dT18:00:00Z)"
LOCATOR="SMK${TS: -5}"

jget() {
  python3 -c 'import json,sys; d=json.load(sys.stdin); print(eval("d"+sys.argv[1]))' "$1"
}

api() {
  local method="$1" path="$2" token="${3:-}" body="${4:-}"
  local args=(-sS -X "$method" "$BASE$path" -H "Content-Type: application/json")
  [[ -n "$token" ]] && args+=(-H "Authorization: Bearer $token")
  [[ -n "$body" ]] && args+=(-d "$body")
  curl "${args[@]}"
}

step() { printf '\n== %s ==\n' "$1"; }

step "Health"
curl -fsS "$BASE/readyz"
echo

step "Frontend"
INDEX=$(curl -fsS "$BASE/")
[[ "$INDEX" == *"/assets/js/app.js"* ]]
[[ "$(curl -sS -o /dev/null -w '%{http_code}' "$BASE/assets/styles.css")" == "200" ]]
[[ "$(curl -sS -o /dev/null -w '%{http_code}' "$BASE/assets/js/app.js")" == "200" ]]
echo "SPA y assets disponibles"

step "Registro de viajero"
REG=$(api POST /auth/register "" "{\"email\":\"$TRAVELER\",\"full_name\":\"Smoke Traveler\",\"password\":\"$PASSWORD\"}")
TOKEN=$(printf '%s' "$REG" | jget "['tokens']['access_token']")
echo "usuario creado: $TRAVELER"

step "Crear reserva manual (genera viaje automáticamente)"
RES=$(api POST /reservations "$TOKEN" "{\"type\":\"hotel\",\"provider\":\"mockhotel\",\"provider_locator\":\"$LOCATOR\",\"title\":\"Hotel Smoke\",\"destination\":\"Bariloche\",\"start_at\":\"$START\",\"end_at\":\"$END\",\"status\":\"confirmed\",\"details\":{\"rooms\":1}}")
TRIP_ID=$(printf '%s' "$RES" | jget "['trip_id']")
RES_ID=$(printf '%s' "$RES" | jget "['id']")
echo "reserva=$RES_ID viaje=$TRIP_ID"

step "Listar viajes y compartir"
TRIPS=$(api GET /trips "$TOKEN")
TRIP_COUNT=$(printf '%s' "$TRIPS" | jget "['data'].__len__()")
echo "viajes activos: $TRIP_COUNT"
api POST "/trips/$TRIP_ID/shares" "$TOKEN" "{\"guest_email\":\"$GUEST\"}" >/dev/null
echo "compartido con $GUEST"

step "Vincular correo, agregar mensaje mock y escanear"
api POST /mail/accounts "$TOKEN" '{"provider":"mock","email":"smoke@example.com","access_token":"mock-token","scopes":["mail.read"]}' >/dev/null
curl -sS -X POST "$BASE/mock/mail/smoke@example.com/messages" \
  -H "Content-Type: application/json" -H "X-Mock-Key: $MOCK_KEY" \
  -d "{\"from\":\"reservas@aerolineas.com\",\"subject\":\"Vuelo confirmado\",\"body\":\"Provider: aerolineas\\nLocator: $LOCATOR\\nType: flight\\nTitle: Vuelo Buenos Aires - Bariloche\\nDestination: Bariloche\\nStart: $START\\nEnd: $END\"}" >/dev/null
ACCOUNT_ID=$(api GET /mail/accounts "$TOKEN" | jget "['data'][0]['id']")
api POST "/mail/accounts/$ACCOUNT_ID/scan" "$TOKEN" >/dev/null
sleep 4
EVENTS=$(api GET /mail/events "$TOKEN")
PROCESSED=$(printf '%s' "$EVENTS" | python3 -c 'import json,sys; d=json.load(sys.stdin)["data"]; print(sum(1 for e in d if e["status"] in ("processed","ignored")))')
echo "eventos de ingesta finalizados: $PROCESSED"

step "Agencia favorita y recomendaciones"
AGENCIES=$(api GET /agencies "$TOKEN")
AGENCY_ID=$(printf '%s' "$AGENCIES" | jget "['data'][0]['id']")
api PUT /agencies/favorite "$TOKEN" "{\"agency_id\":\"$AGENCY_ID\"}" >/dev/null
RECS=$(api GET /recommendations "$TOKEN")
REC_ID=$(printf '%s' "$RECS" | jget "['data'][0]['id']")
api POST "/recommendations/$REC_ID/inquire" "$TOKEN" '{"message":"Quisiera cotizar esta propuesta"}' >/dev/null
echo "consulta enviada por recomendación $REC_ID"

step "Sincronización mock de proveedor"
curl -sS -X PUT "$BASE/mock/providers/mockhotel/reservations/$LOCATOR" \
  -H "Content-Type: application/json" -H "X-Mock-Key: $MOCK_KEY" \
  -d "{\"type\":\"hotel\",\"title\":\"Hotel Smoke (actualizado)\",\"destination\":\"Bariloche\",\"start_at\":\"$START\",\"end_at\":\"$END\",\"status\":\"confirmed\",\"details\":{\"rooms\":2}}" >/dev/null
api POST "/reservations/$RES_ID/sync" "$TOKEN" >/dev/null
echo "reserva sincronizada desde proveedor mock"
curl -sS -X POST "$BASE/mock/providers/mockhotel/fail" \
  -H "Content-Type: application/json" -H "X-Mock-Key: $MOCK_KEY" \
  -d '{"enabled":true}' >/dev/null
STALE=$(api POST "/reservations/$RES_ID/sync" "$TOKEN" | jget "['stale']")
echo "reserva marcada stale ante caída del proveedor: $STALE"
[[ "$STALE" == "True" ]]
curl -sS -X POST "$BASE/mock/providers/mockhotel/fail" \
  -H "Content-Type: application/json" -H "X-Mock-Key: $MOCK_KEY" \
  -d '{"enabled":false}' >/dev/null
RECOVERED=$(api POST "/reservations/$RES_ID/sync" "$TOKEN" | jget "['stale']")
echo "reserva recuperada tras volver el proveedor: $RECOVERED"
[[ "$RECOVERED" == "False" ]]

step "Social preview y publicación"
api POST "/trips/$TRIP_ID/social/preview" "$TOKEN" '{"network":"x"}' | jget "['text']"
api POST "/trips/$TRIP_ID/social/share" "$TOKEN" '{"network":"x"}' >/dev/null

step "Invitado ve el viaje compartido"
GUEST_REG=$(api POST /auth/register "" "{\"email\":\"$GUEST\",\"full_name\":\"Smoke Guest\",\"password\":\"$PASSWORD\"}")
GUEST_TOKEN=$(printf '%s' "$GUEST_REG" | jget "['tokens']['access_token']")
SHARED=$(api GET /shared-with-me "$GUEST_TOKEN")
SHARED_COUNT=$(printf '%s' "$SHARED" | jget "['data'].__len__()")
echo "viajes compartidos visibles para invitado: $SHARED_COUNT"
[[ "$SHARED_COUNT" -ge 1 ]]
api GET "/shared/$TRIP_ID" "$GUEST_TOKEN" >/dev/null

step "Analítica (rol analyst)"
ANALYST_TOKEN=$(api POST /auth/login "" '{"email":"analyst@trotamundos.dev","password":"analyst12345"}' | jget "['tokens']['access_token']")
SUMMARY=$(api GET /analytics/summary "$ANALYST_TOKEN")
TOTAL=$(printf '%s' "$SUMMARY" | jget "['reservations']")
echo "reservas anonimizadas en analítica: $TOTAL"

step "OK"
