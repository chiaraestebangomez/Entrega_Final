# Trotamundos

Plataforma de viajes que centraliza en un solo lugar las reservas dispersas en correos y proveedores: agrupa automáticamente los viajes, permite compartirlos en modo solo lectura, ofrece recomendaciones de una agencia favorita y expone métricas anonimizadas para analistas.

El entregable es un **prototipo funcional** desarrollado con las decisiones de las cinco iteraciones del método ADD 3.0. La [wiki del proyecto](https://github.com/chiaraestebangomez/Entrega_Final/wiki) es la fuente de verdad: cuando allí se describe infraestructura de producción (Patroni, CDN, sharding efectivo, réplica OLAP), se trata como **objetivo de diseño**, no como parte del prototipo.

> **Estructura:** este README está en la raíz del repositorio. Todo el código, el `Makefile`, la configuración y los contenedores están en [`trotamundos/`](trotamundos/).

## Inicio rápido

Requisito: Docker con Compose v2. No hace falta instalar Go, PostgreSQL ni Redis.

```bash
git clone https://github.com/chiaraestebangomez/Entrega_Final.git
cd Entrega_Final/trotamundos
make                       # target por defecto: alias de `make up`
```

`make up` ejecuta `docker compose up --build -d --wait`: levanta PostgreSQL 16, Redis 7 y la API, y espera a que los *health checks* estén OK antes de devolver el control. Las claves RSA se generan automáticamente la primera vez en el volumen `keys`.

| Servicio | Dirección |
| :--- | :--- |
| SPA y API | <http://localhost:8080> |
| Estado | `GET /healthz`, `GET /readyz` |
| PostgreSQL | `localhost:5432` (`trotamundos` / `trotamundos`) |
| Redis | `localhost:6379` |

Los viajeros se registran desde la propia SPA o con `POST /auth/register`. El perfil de analista del compose es `analyst@trotamundos.dev` / `analyst12345`.

Para bajar los servicios: `make down` (conserva volúmenes) o `make clean` (borra contenedores y volúmenes).

### Verificación

```bash
make test       # go test ./...
make smoke      # prueba de humo contra http://localhost:8080 (API levantada)
make test-e2e   # limpia, levanta, corre tests y smoke, y vuelve a apagar todo
```

El smoke cubre registro y login, reservas y agrupación de viajes, compartición, ingesta simulada de correo, agencia favorita y recomendaciones, sincronización con proveedor (incluido el estado `stale` cuando falla), publicación social y analítica. `make test-e2e` termina con los contenedores apagados; si querés seguir usando la app, ejecutá `make` otra vez.

### Ejecución local sin Docker

Necesitás Go 1.22, PostgreSQL 16 y Redis 7 escuchando en `localhost`. El binario **no lee archivos `.env` automáticamente**: las variables se toman del entorno del proceso.

```bash
cd trotamundos
set -a; source .env.example; set +a    # valores de desarrollo
make run                              # go run ./cmd/api
```

## Qué incluye

- **Viajeros:** alta de reservas manuales, ingesta de correos, agrupación automática de viajes, detalle de itinerario, notificaciones y historial de cambios.
- **Compartición:** invitaciones por correo con acceso de solo lectura y consentimiento explícito para usar la ubicación del viaje.
- **Agencia y recomendaciones:** agencia favorita, preferencias de presupuesto, señales anonimizadas de feedback, *cold start* desde el catálogo de la agencia y consultas registradas con estado de entrega.
- **Analítica:** hechos sin identificadores personales, consultas por rango de fechas con rol `analyst` o `admin`.
- **Seguridad:** JWT RS256 con `jti`, refresco opaco con rotación atómica en Redis, denylist de tokens revocados, RBAC, cifrado AES-GCM de tokens de correo, API key por cliente y limitación de tasa por IP y endpoint.
- **SPA:** embebida en el binario con `go:embed` (HTML, CSS y JavaScript sin compilación ni npm). Se sirve con `Cache-Control: no-cache`; la CDN de la Iteración 3 es objetivo de producción.

## Arquitectura

- Go 1.22, monolito modular con `net/http` y patrones de rutas de Go 1.22 (sin framework HTTP).
- PostgreSQL 16 con esquemas `auth`, `itinerary` y `analytics`; Redis 7 para refresco, revocación y rate limiting.
- Adaptadores de proveedores simulados (con reintentos y circuit breaker) y conector de correo simulado; no se hacen llamadas externas reales.
- Docker multietapa + Docker Compose.

```text
cmd/api                  composición, workers y servidor HTTP
internal/
  api                    router y middleware
  auth                   usuarios, login, refresh, denylist y RBAC
  itinerary              viajes, reservas, agrupación, sync y auditoría
  ingestion              parser de correos, filtros, outbox y worker
  providers              adaptadores mock con reintentos y circuit breaker
  mailer                 conector de correo mock
  analytics              publicación y consulta de hechos anonimizados
  agencies               agencia favorita, recomendaciones y consultas
  sharing                compartición de viajes por correo
  social                 vista previa y publicación mock en redes
  web                    SPA embebida, estilos, JavaScript y assets
  platform/              pgx, Redis, JWT, cifrado, breaker, geo y httpx
api/                     contratos OpenAPI y AsyncAPI
scripts/smoke.sh         prueba de humo end-to-end
```

Decisiones que conviene conocer:

- **Agrupación:** mismo destino (comparación tolerante) y ventana de ±3 días; se elige el viaje más cercano.
- **Precedencia de datos:** proveedor > manual > correo al fusionar reservas.
- **Idempotencia:** deduplicación por `(user_id, provider, provider_locator)` y por `(user_id, message_id)` en ingesta.
- **Asincronía:** outbox en PostgreSQL consumido con `FOR UPDATE SKIP LOCKED`, reintentos exponenciales y recuperación de eventos trabados.
- **Analítica:** los hechos se publican sin `user_id`, correo ni localizadores; la lectura usa un pool de solo lectura.
- **Revocación:** el logout de la SPA envía el bearer para revocar el `jti` cuando el access sigue vigente; si expiró, el servidor lo ignora y no se refresca, pero el refresh token sí se revoca.
- **Fragmentación:** `pgxdb.Shards` y `DATABASE_SHARDS` dejan preparada la selección de pool por `user_id`, pero ningún repositorio la usa todavía: el prototipo opera con un shard.

## Configuración

Las variables y sus valores de desarrollo están en [`trotamundos/.env.example`](trotamundos/.env.example). Las principales:

| Variable | Por defecto | Descripción |
| :--- | :--- | :--- |
| `HTTP_ADDR` | `:8080` | Dirección de escucha HTTP |
| `APP_ENV` | `development` | En `production` se exigen secretos reales |
| `DATABASE_URL` | PostgreSQL local | DSN principal |
| `DATABASE_SHARDS` | `DATABASE_URL` | Lista de DSN separada por comas |
| `OLAP_DATABASE_URL` | primer shard | DSN de solo lectura para analítica |
| `REDIS_ADDR` | `localhost:6379` | Dirección de Redis |
| `REDIS_PASSWORD` / `REDIS_DB` | vacío / `0` | Credenciales de Redis |
| `JWT_PRIVATE_KEY_PATH` / `JWT_PUBLIC_KEY_PATH` | `keys/jwt_*.pem` | Claves RSA; se generan si no existen |
| `JWT_ISSUER` | `trotamundos` | Emisor del access token |
| `JWT_ACCESS_TTL` | `15m` | Vigencia del access token |
| `REFRESH_TTL` | `720h` | Vigencia del refresh token (30 días) |
| `TOKEN_ENCRYPTION_KEY` | clave de demostración | Base64 de 32 bytes para AES-GCM |
| `MOCK_CONTROL_KEY` | `dev-mock-key` | `X-Mock-Key` para los controladores simulados |
| `WEBHOOK_KEY` | `dev-webhook-key` | `X-Webhook-Key` del webhook de correo |
| `API_KEYS` | vacío | Claves `X-API-Key` (CON-01); vacío desactiva el chequeo en desarrollo |
| `WEB_API_KEY` | primera de `API_KEYS` | Clave pública que la SPA lee de `GET /api-config.js` |
| `RATE_LIMIT_PER_MINUTE` / `RATE_LIMIT_WINDOW` | `300` / `1m` | Cuota por IP y endpoint |
| `TRUSTED_PROXIES` | vacío | IPs o CIDRs a los que se confía `X-Forwarded-For` |
| `ENABLE_WORKERS` | `true` | Activa ingesta, sincronización y archivado |
| `INGESTION_POLL_INTERVAL` | `2s` | Sondeo del outbox |
| `PROVIDER_SYNC_INTERVAL` | `2m` | Sincronización automática de proveedores |
| `AUTO_ARCHIVE_INTERVAL` | `10m` | Archivado de viajes pasados |
| `STALE_EVENT_AFTER` | `2m` | Antigüedad para recuperar eventos trabados |
| `RECOMMENDATION_*` | ver `.env.example` | TTLs de resultados, snapshot, pool, lock y ventana de señales |
| `SHARD_COUNT` | `1` | Cantidad de shards declarada |
| `SEED_ANALYST_EMAIL` / `SEED_ANALYST_PASSWORD` | vacío | Analista inicial (el compose los fija) |
| `CORS_ALLOWED_ORIGIN` | `*` | Origen permitido |

Con `APP_ENV=production` la carga falla si no se definen `TOKEN_ENCRYPTION_KEY`, `MOCK_CONTROL_KEY`, `WEBHOOK_KEY` y `API_KEYS` con valores reales, o si CORS queda en `*`. El perfil del compose es de desarrollo: no exige API key y usa secretos de ejemplo.

## API y contratos

Especificaciones: [`api/openapi.yaml`](trotamundos/api/openapi.yaml) y [`api/asyncapi.yaml`](trotamundos/api/asyncapi.yaml). Los tests validan que ambas parsean y que OpenAPI coincide con las rutas del router. Salvo las rutas públicas, se requiere `Authorization: Bearer <token>`; el logout admite el bearer para revocar también el access token.

- **Rutas públicas:** `GET /`, `GET /assets/...`, `GET /api-config.js` y los health checks; también `/auth/*`, `/mock/*`, `/webhooks/mail` y `GET /social/networks` (estas últimas con `X-Mock-Key`/`X-Webhook-Key` según corresponda).
- **Autenticación:** `POST /auth/register`, `POST /auth/login`, `POST /auth/refresh`, `POST /auth/logout`, `GET /auth/me`.
- **Itinerario:** `GET /trips`, `GET|DELETE /trips/{id}`, `POST /trips/{id}/reservations`, `POST /reservations`, `GET|PATCH|DELETE /reservations/{id}`, `POST /reservations/{id}/sync`, `GET /reservations/{id}/changes`, notificaciones y `POST /trips/{id}/context-location`.
- **Correo:** cuentas, eventos, listas blancas y filtros bajo `/mail/...`, más `POST /webhooks/mail` con `X-Webhook-Key`.
- **Agencia:** `GET /agencies`, `GET|PUT|DELETE /me/favorite-agency`, `GET|PUT /me/preferences`, `GET /recommendations`, `POST /recommendations/{id}/feedback`, `POST /recommendations/{id}/info-request` y consultas a agencia. Hay alias compatibles bajo `/agencies/favorite`, `/discard` y `/inquire`.
- **Compartición y social:** `/shares`, `/shared-with-me`, `/shared/{tripId}`, `GET /social/networks`, `POST /trips/{id}/social/preview` y `POST /trips/{id}/social/share`.
- **Analítica** (rol `analyst` o `admin`): `GET /analytics/summary` y `GET /analytics/facts`, con `from` y `to` en `YYYY-MM-DD` (también acepta RFC3339).

## Controles de simulación

Requieren `X-Mock-Key: <MOCK_CONTROL_KEY>`:

- `POST /mock/mail/{email}/messages`: agrega un correo a la bandeja simulada.
- `PUT /mock/providers/{provider}/reservations/{locator}`: crea o actualiza la instantánea de una reserva.
- `DELETE /mock/providers/{provider}/reservations/{locator}`: la marca como cancelada.
- `POST /mock/providers/{provider}/fail`: `{"enabled":true}` simula una caída del proveedor.

Los correos se procesan por *polling* cada `INGESTION_POLL_INTERVAL`; se puede forzar un ciclo con `POST /mail/accounts/{id}/scan`.

## Limitaciones conocidas

- Los adaptadores son mocks: no hay OAuth real de Gmail/Outlook ni APIs directas de aerolíneas o hoteles. La publicación social también es simulada.
- La analítica usa la misma base por defecto; `OLAP_DATABASE_URL` permite apuntar a una réplica, pero el prototipo sólo aísla lógicamente con un pool de solo lectura, sin réplica física ni replicación < 1 s.
- No hay alta disponibilidad de infraestructura: un solo Postgres, un solo Redis y una sola instancia de la API. Patroni/DCS, standby con RPO=0, ALB/NGINX, CDN, clúster Redis y auto-scaling 10× son decisiones de la Iteración 3, no partes del entregable.
- El modelo de recomendación es heurístico: no hay un modelo entrenado externo ni métricas de calidad medidas.
- Las latencias objetivo (≤ 800 ms en web, ≤ 50 ms en validación de token) y la degradación ≤ 5 % se persiguen con tests y timeouts, pero no hay benchmarks de carga que las certifiquen.

## Referencias

- [Wiki del proyecto](https://github.com/chiaraestebangomez/Entrega_Final/wiki): historias de usuario, escenarios QA, restricciones, ADR e iteraciones del ADD.
- [Escenarios de calidad](https://github.com/chiaraestebangomez/Entrega_Final/wiki/QA)
- [ADRs](https://github.com/chiaraestebangomez/Entrega_Final/wiki)
