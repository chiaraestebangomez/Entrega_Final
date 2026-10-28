# Trotamundos

Plataforma Trotamundos con backend Go y frontend web: los viajeros centralizan reservas dispersas en emails y proveedores, reciben un itinerario agrupado automáticamente, comparten viajes de solo lectura y acceden a recomendaciones de una agencia favorita. Los analistas consultan métricas anonimizadas en una base separada de lectura.

La implementación sigue los artefactos de la wiki del proyecto (historias de usuario, QAS, restricciones, ADR e iteraciones del método ADD 3.0).

## Pila
* Go 1.22, monolito modular con paquetes por dominio.
* PostgreSQL 16: esquemas `auth`, `itinerary`, `analytics`.
* Redis 7: actualiza tokens con rotación.
* JWT RS256 (15 min) + refresco opaco (30 días).
* Sin dependencias de framework HTTP: `net/http` con patrones de Go 1.22.
* Adaptadores de proveedores y correo simulados; no se realizan llamadas externas.
* Frontend SPA embebido en el binario Go: HTML, CSS y JavaScript sin compilación ni dependencias externas.
* Docker multietapa + Docker Compose.

---


## Interfaz
La SPA se sirve desde la misma API y se empaqueta con `go:embed` desde `internal/web/static`. No requiere npm, CDN ni un servidor adicional.

* Resumen, viajes y detalle de itinerario, viajes compartidos, correo, agencia, notificaciones y análisis.
* Iniciar sesión/registro con token de acceso y actualización automática.
* Creación y edición de reservas, compartición por email, consultas a agencias, ingesta simulada de emails y publicación social simulada.
* Navegación por hash: `#/dashboard`, `#/trips`, `#/mail`, `#/agency`, `#/analytics`, entre otras.

---


**Requisitos:** Go 1.22, PostgreSQL 16 y Redis 7.

```bash
cp .env.example .env
set -a && source .env && set +a
go mod download
go run ./cmd/api
```

Las migraciones y semillas se aplican automáticamente al iniciar. Las claves RSA se generan en `keys/` si no existen.

---

## Iniciar el sistema

```bash
git clone https://github.com/chiaraestebangomez/Entrega_Final
cd Entrega_Final
cd trotamundos
docker compose up --build -d
curl http://localhost:8080/readyz
```

Abra `http://localhost:8080` para usar la aplicación.

**Usuario analista sembrado por defecto:**
`analyst@trotamundos.dev` / `analyst12345`

**Prueba de humo de extremo a extremo:**
```bash
make test-e2e
```

**Cobertura funcional incluida:** analizador de correos, filtros, agrupación por destino/fechas, autenticación con rotación de refresco, JWT, cifrado AES-GCM, proveedores simulados, analítica anonimizada, social, compartición y entrega de la SPA con sus activos.

---

## Arquitectura

```text
cmd/api                  composición, workers y servidor HTTP
internal/
  api                    router y middleware
  auth                   usuarios, login, refresh tokens, RBAC
  itinerary              viajes, reservas, agrupación, sync, auditoría
  ingestion              parser de emails, filtros, outbox y worker
  providers              adapters mock con reintentos y circuit breaker
  mailer                 conector de correo mock
  analytics              publicación y consulta de hechos anonimizados
  agencies               agencia favorita, recomendaciones y consultas
  sharing                compartición de viajes por email
  social                 vista previa y publicación mock en redes
  web                    SPA embebida, estilos, JavaScript y assets
  platform/              pgx, redis, JWT, cifrado, breaker, geo, httpx
```

### Decisiones relevantes:
* **Monolito modular**: límites por paquete y dependencias explícitas.
* **Agrupación automática**: mismo destino (comparación tolerante) y ventana de ±3 días; se elige el viaje más cercano.
* **Precedencia de datos**: proveedor > email > manual al fusionar reservas.
* **Idempotencia**: deduplicación por `(user_id, provider, provider_locator)` y por `(user_id, message_id)` en ingesta.
* **Asincronía**: la ingesta usa una tabla outbox con `FOR UPDATE SKIP LOCKED`, reintentos exponenciales y recuperación de eventos trabados.
* **Analítica aislada**: se escribe en `analytics.reservation_facts` sin ID de usuario, correo electrónico ni localizadores; la lectura usa un pool con transacciones de solo lectura.
* **Privacidad**: tokens de correo cifrados con AES-256-GCM; la compartición exige usuario identificado; el resumen social omite localizadores y datos personales.
* **Resiliencia**: tiempos de espera HTTP, reintentos y disyuntores en adaptadores, trabajadores con recuperación de eventos obsoletos.
* **Fragmentación preparada**: `pgxdb.Shards` enruta por `user_id`; por defecto hay un fragmento.

---

## Variables de entorno

| Variable | Por defecto | Descripción |
| :--- | :--- | :--- |
| `HTTP_ADDR` | `:8080` | Dirección HTTP |
| `APP_ENV` | `development` | `development` usa logs de texto |
| `DATABASE_URL` | PostgreSQL local | Director de DSN |
| `DATABASE_SHARDS` | `DATABASE_URL` | Lista separada por comas de shards |
| `OLAP_DATABASE_URL` | fragmento de imprimación | DSN de solo lectura para analítica |
| `REDIS_ADDR` | `localhost:6379` | Redis |
| `REDIS_PASSWORD`/`REDIS_DB` | vacío / `0` | Credenciales Redis |
| `JWT_PRIVATE_KEY_PATH`/`JWT_PUBLIC_KEY_PATH` | `keys/jwt_*.pem` | Claves RSA |
| `JWT_ISSUER` | `trotamundos` | Emisor del token de acceso |
| `JWT_ACCESS_TTL` | `15m` | Vida del token de acceso |
| `REFRESH_TTL` | `720h` | Vida del refresh token |
| `TOKEN_ENCRYPTION_KEY` | demostración de clave | Base64 de 32 bytes para AES-GCM |
| `MOCK_CONTROL_KEY` | `dev-mock-key` | `X-Mock-Key` Puntos finales simulados del encabezado |
| `WEBHOOK_KEY` | `dev-webhook-key` | Encabezado `X-Webhook-Key` para webhook de correo |
| `ENABLE_WORKERS` | `true` | Activa ingesta, sincronización y archivado |
| `INGESTION_POLL_INTERVAL` | `2s` | Sondeo de la bandeja de salida |
| `STALE_EVENT_AFTER` | `2m` | Antigüedad para recuperar eventos trabados |
| `PROVIDER_SYNC_INTERVAL` | `2m` | Sincronización automática de proveedores |
| `AUTO_ARCHIVE_INTERVAL` | `10m` | Archivado de viajes pasados |
| `SHARD_COUNT` | `1` | Cantidad de fragmentos declarada |
| `SEED_ANALYST_EMAIL`/`SEED_ANALYST_PASSWORD` | vacío | Analista inicial |
| `CORS_ALLOWED_ORIGIN` | `*` | Origen permitido |

---

## API

Contratos: `api/openapi.yaml` y `api/asyncapi.yaml`.

### Interfaz
* `GET /`: sirve la SPA.
* `GET /assets/...`: sirve estilos, scripts e iconos embebidos.

### Autenticación
* `POST /auth/register`, `POST /auth/login`, `POST /auth/refresh`, `POST /auth/logout`
* `GET /auth/me`
* Token de acceso: `Authorization: Bearer <token>`.

### Itinerario
* `GET /trips?archived=true|false`
* `GET /trips/{id}`, `DELETE /trips/{id}?confirm=true`
* `POST /trips/{id}/reservations`, `POST /reservations`
* `GET|PATCH|DELETE /reservations/{id}` (`confirm=true` para borrar)
* `POST /reservations/{id}/sync`
* `GET /reservations/{id}/changes`
* `GET /notifications`, `POST /notifications/{id}/read`
* `POST /trips/{id}/context-location` (consentimiento + coordenadas)

### Ingesta de correo
* `GET|POST /mail/accounts`, `DELETE /mail/accounts/{id}`, `POST /mail/accounts/{id}/scan`
* `GET /mail/events`
* `GET|POST /mail/whitelist`, `DELETE /mail/whitelist/{id}`
* `GET|POST /mail/filters`, `PATCH|DELETE /mail/filters/{id}`
* `POST /webhooks/mail` (encabezado `X-Webhook-Key`)

### Agencias, compartición y social
* `GET /agencies`, `GET|PUT|DELETE /agencies/favorite`
* `GET /recommendations`, `POST /recommendations/{id}/discard`, `POST /recommendations/{id}/inquire`
* `POST /reservations/{id}/agency-request`
* `GET /agency-requests`, `GET /agency-requests/{id}`, `POST /agency-requests/{id}/close`
* `GET|POST /trips/{id}/shares`, `DELETE /shares/{id}`
* `GET /shared-with-me`, `GET /shared/{tripId}`
* `GET /social/networks`, `POST /trips/{id}/social/preview`, `POST /trips/{id}/social/share`

### Analítica (`analyst` o `admin`)
* `GET /analytics/summary?from=YYYY-MM-DD&to=YYYY-MM-DD`
* `GET /analytics/facts?from=...&to=...`

---

## Simulación de controles

Requieren `X-Mock-Key: <MOCK_CONTROL_KEY>`.

* `POST /mock/mail/{email}/messages`: agrega un correo electrónico a la bandeja simulada.
* `PUT /mock/providers/{provider}/reservations/{locator}`: crea/actualiza la instantánea de una reserva.
* `DELETE /mock/providers/{provider}/reservations/{locator}`: marca la reserva como cancelada.
* `POST /mock/providers/{provider}/fail`: `{"enabled":true}` simula caída del proveedor.

Los correos electrónicos se procesan por polling cada `INGESTION_POLL_INTERVAL`; se puede forzar un ciclo con `POST /mail/accounts/{id}/scan`.

---

## Limitaciones conocidas
* Los adaptadores de proveedores son mocks simulados; no hay OAuth real de Gmail/Outlook ni API directa de aerolíneas/hoteles.
* La compartición social devuelve una publicación simulada.
* El sharding está modelado y es configurable, pero la selección de pool por usuario aún no se aplica en los repositorios (un shard por defecto).
* La analítica usa la misma base por defecto; para separar OLTP/OLAP configurar `OLAP_DATABASE_URL` con una réplica.
