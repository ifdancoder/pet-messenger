# pet-messenger

*[English version](README.md)*

Backend (и небольшой React-клиент) для мессенджера в реальном времени:
личные сообщения, групповые чаты в стиле Slack и broadcast-каналы в
стиле Telegram, групповые аудио/видеозвонки с демонстрацией экрана
через собственный WebRTC SFU, веб-push-уведомления и end-to-end
зашифрованный обмен ключами для звонков.

Backend разделён на два сервиса в одном Go-модуле: `api-service`
(stateless: auth, чаты, каналы, уведомления, WebSocket fan-out) и
`media-service` (stateful: сам Pion SFU, один звонок живёт на одном
поде всё время своего существования). Они построены как ретрофит на
Clean Architecture + Hexagonal + CQRS, организованный в 8 независимых
bounded contexts.

## Стек технологий

- Go 1.26, Gin (HTTP), `uber-go/fx` (composition root / dependency injection)
- PostgreSQL через `database/sql` + `pgx`, `golang-migrate` для миграций схемы
- Redis (отслеживание присутствия, кэш), NATS JetStream (durable, at-least-once fan-out событий между подами)
- Pion WebRTC (сам SFU) + coturn (TURN/STUN relay) для звонков
- MinIO (S3-совместимое хранилище) для аватаров
- gRPC (`proto/media/v1/media.proto`) как control plane между `api-service` и `media-service`
- Swagger/OpenAPI через `swaggo/swag`, раздаётся на `/swagger` при `SWAGGER_ENABLED=true`
- Фронтенд на React 19 + Vite + TypeScript
- Docker Compose для локальной разработки; Kubernetes (Kustomize: `base` + оверлеи `minikube`/`production`) для деплоя
- GitHub Actions: `ci.yml` тестирует каждый PR, `deploy.yml` билдит, пушит и деплоит на общий k3s-узел при каждом push в `main`
- `testcontainers-go` для интеграционных тестов (поднимает настоящие контейнеры Postgres/NATS/Redis; нужен Docker)

## Ключевые инженерные решения

- **Clean + Hexagonal + CQRS, пакеты по bounded context.** Каждый из
  8 контекстов (`auth`, `user`, `conversation`, `message`, `pushtoken`,
  `notification`, `e2ee`, `call`) это своё дерево
  `internal/<context>/{domain,application/{command,query,port/{in,out}},adapter/{in,out}}`.
  Сквозная инфраструктура (composition root на fx, конфиг, общие
  transport- и infra-адаптеры) лежит отдельно от всех них, в
  `internal/platform/` (см. собственный `README.md` этой папки).
- **Порты принадлежат потребителю.** Интерфейс лежит в пакете, который
  его вызывает, а не в пакете, который его реализует. Контекст, которому
  нужны данные другого контекста, идёт через собственный узкий порт
  этого другого контекста, а не через его внутренности.
- **Ключи звонков в e2ee через поручительство устройств, а не
  доверенный key-сервер.** Устройство становится доверенным, когда уже
  доверенное устройство того же аккаунта подписывает его регистрацию;
  ключ сессии звонка оборачивается отдельно для каждого доверенного
  устройства-получателя.
- **Идемпотентная отправка сообщений.** `SendMessageCommand` принимает
  idempotency key от клиента, поэтому повторный запрос из-за нестабильного
  соединения не создаёт сообщение дважды.
- **Keyset (курсорная) пагинация везде, где есть списки** (беседы,
  сообщения, участники, уведомления, публичные каналы), а не offset-
  пагинация, чтобы страница оставалась корректной при конкурентных записях.
- **Разделение управления ресурсами на общем узле по тому, кто может
  его менять.** `resources.requests/limits` на уровне контейнера лежат в
  `deploy/kubernetes/base/` и деплоятся через собственный CI проекта.
  `ResourceQuota`/`LimitRange`, ограничивающий весь namespace снаружи,
  лежит в `cluster/quotas/` и применяется руками под kubeconfig админа
  кластера, намеренно так, чтобы собственный пайплайн проекта не мог
  редактировать потолок, который его же ограничивает.

## Структура проекта

- `cmd/`: бинарники `api`, `media`, `migrate`, `genkey`, `rebuildconversationinbox`.
- `internal/<context>/`: 8 bounded context'ов, у каждого свои слои domain/application/adapter.
- `internal/platform/`: composition root (`fx/`), `config/`, бутстрап `database/`, общий/generic `adapter/` и `infrastructure/`.
- `db/migrations/`: SQL-миграции `golang-migrate`.
- `proto/`: gRPC-контракт `RoomControl` между `api-service` и `media-service`.
- `frontend/`: клиент на React + Vite + TypeScript.
- `deploy/`: стеки Docker Compose (корневой и `deploy/media/` для полного call-стека) и `deploy/kubernetes/` (манифесты Kustomize).
- `cluster/`: конфигурация k8s на весь кластер (квоты, cert-manager), применяется руками, никогда через CI отдельного проекта.
- `docs/`: дизайн-спеки и планы (`docs/superpowers/`) и `docs/deploy-guide.md`, руководство по продакшен-деплою.

## Как запустить

`cmd/api` при старте подключается к Postgres, Redis, NATS и MinIO и не
поднимается, если хотя бы один из них недоступен (проверка идёт сразу,
а не лениво при первом использовании). Режима "только Postgres" не
существует. Сначала поднимите все четыре:

```bash
cp .env.example .env
# заполните хотя бы JWT_ED25519_SEED_BASE64 (go run ./cmd/genkey)
docker compose up -d psql_bp                    # только Postgres; см. примечание ниже
docker run -d --name pm-redis --network host redis:7 --port 6379
docker run -d --name pm-nats  --network host nats:2.10 -js
docker run -d --name pm-minio --network host -e MINIO_ROOT_USER=minioadmin \
  -e MINIO_ROOT_PASSWORD=minioadmin minio/minio server /data
go run ./cmd/migrate -direction up
make run   # go run cmd/api/main.go, плюс dev-сервер фронтенда на :5173
```

`make docker-run` (полный стек `app` + `frontend` + `psql_bp` из
корневого `docker-compose.yml`) старше, чем добавление NATS/Redis/MinIO/
звонков в проект, и не передаёт их в контейнер `app`. Используйте его
только для `psql_bp`, а не для рабочего контейнеризованного `app`. См.
раздел "Известные ограничения" ниже.

### Полный стек, включая звонки

```bash
export JWT_ED25519_SEED_BASE64="$(go run ./cmd/genkey)"
docker compose -f deploy/media/docker-compose.yml up --build
cd frontend && npm install && npm run dev
```

`deploy/media/docker-compose.yml` тоже не поднимает MinIO, поэтому
загрузка/отдача аватаров там не будет работать по той же причине; всё
остальное (чаты, каналы, звонки) работает. Процедура ручной проверки
звонка в двух вкладках (включая принудительный TURN relay) описана в
`deploy/media/README.md`, а путь через Minikube, который использует те
же манифесты, что и production-оверлей, и включает MinIO, описан в
`deploy/kubernetes/README.md`.

### Другие команды Makefile

`make build` (только бинарник), `make test` (весь набор тестов),
`make itest` (только интеграционный набор для Postgres), `make watch`
(live reload через `air`), `make clean`.

## Переменные окружения и секреты

Полный список с комментариями о том, что обязательно, что имеет
дефолт, и где взять настоящее значение, лежит в `.env.example`. Никогда не
кладите настоящие значения в этот файл и не коммитьте их; сам `.env`
в `.gitignore`.

Основное:

- `JWT_ED25519_SEED_BASE64` (обязателен): `go run ./cmd/genkey`.
- `BLUEPRINT_DB_*`: подключение к вашему Postgres.
- `PUSH_VAPID_*` / `PUSH_FCM_*` (опционально): оставьте пустыми, чтобы отключить web push / FCM push.
- `MEDIA_RPC_TOKEN`, `TURN_SECRET`: общие секреты между `api`, `media` и coturn. Как генерировать и хранить их для настоящего деплоя (не оставлять dev-дефолтами из репозитория), написано в `docs/deploy-guide.md`.

## Документация API

REST API задокументирован через Swagger (OpenAPI 2.0), генерируется из
`@`-аннотаций над HTTP-контроллером каждого bounded context'а
(`internal/<context>/adapter/in/web/controller.go`). Перегенерировать
спеку после изменения роута:

```bash
make swagger
```

Swagger UI закрыт флагом `SWAGGER_ENABLED=true` (по умолчанию выключен)
и раздаётся на `/swagger/index.html`, когда включён.

Примеры запросов:

```bash
curl -X POST http://localhost:8080/auth/register \
  -H "Content-Type: application/json" \
  -d '{"username": "alice", "password": "a-strong-password", "display_name": "Alice"}'

curl http://localhost:8080/conversations \
  -H "Authorization: Bearer <access_token>"
```

## Миграции базы данных

```bash
go run ./cmd/migrate -direction up    # или -direction down
```

Читает ту же конфигурацию `BLUEPRINT_DB_*`, что и `cmd/api`, применяет
к `db/migrations/`. В Kubernetes тот же бинарник (собранный из
`deploy/kubernetes/Dockerfile.migrate`) запускается как одноразовый
`migrate` Job при каждом деплое; его pod template неизменяем, поэтому
повторный запуск это удаление и пересоздание, а не патч (`deploy.yml`
уже делает это сам).

## Тесты

```bash
make test    # go test ./... -v
make itest   # только internal/platform/database и internal/platform/infrastructure/postgres
```

Большая часть набора это юнит-тесты; пакеты персистентности и messaging
используют `testcontainers-go`, поэтому для полного `make test` нужен
запущенный Docker. В `internal/platform/adapter/sfu` есть несколько
настоящих end-to-end тестов на WebRTC, чувствительных к задержкам
планировщика на загруженной машине, которые иногда падают независимо
от реальных изменений в коде (`TestSFUEndToEnd`,
`TestSFUViewerCannotPublish`, `TestSFUMuteRequiresModerator`,
`TestSessionReconnectPreservesRoomAndOwnership`); перезапуск именно
этого пакета отдельно, это способ отличить настоящую регрессию от флака
по расписанию.

## Известные ограничения

- `make docker-run` / сервис `app` из корневого `docker-compose.yml` не
  поднимается как есть: ему не передан `JWT_ED25519_SEED_BASE64`, и там
  нет сервисов NATS/Redis/MinIO, до которых он мог бы достучаться, даже
  если это исправить. Используйте его только для поднятия `psql_bp`;
  для рабочего контейнеризованного полного стека используйте `deploy/media/docker-compose.yml`.
- Нет staging-окружения; merge в `main` деплоится прямо в продакшен
  (см. `docs/deploy-guide.md`).
- Нет автомасштабирования.
- `cmd/rebuildconversationinbox` запускается оператором вручную
  (`kubectl run`/разовый `Job`), а не как постоянная задача по расписанию.
- Production k3s-узел общий с другим проектом; его собственная
  `ResourceQuota` (`cluster/quotas/other-project.yaml`) пока заглушка в
  ожидании реальных цифр от его администратора.
- Idle timeout на entrypoint Traefik перед production Ingress пока не
  подтверждён как достаточно долгий для тихого звонка с его сигнальным
  WebSocket; см. шаг проверки после деплоя в `docs/deploy-guide.md`.
- Нет тестового набора для фронтенда; CI только линтит (`oxlint`) и
  собирает его.

## Лицензия

В репозитории пока нет ни файла `LICENSE`, ни поля `license` в
манифесте пакета. До тех пор, как что-то из этого появится, все права
по умолчанию защищены (all rights reserved).
