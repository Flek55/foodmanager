# Foodmanager

Семейный список покупок и учёт запасов: общий список в реальном времени,
отметки «заканчивается», штрихкоды, напоминания через Telegram-бота
и прогноз «пора купить» по истории покупок.

Архитектура и план: [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

## Стек

- **Backend:** Go, chi, pgx, sqlc, goose, PostgreSQL 18
- **Frontend:** React, TypeScript, Vite, Tailwind, TanStack Query (PWA и Telegram Mini App)
- **Контракт:** [api/openapi.yaml](api/openapi.yaml): из него генерируется Go-сервер и TS-клиент

## Быстрый старт

Нужны Go 1.26+, Node 24+, Docker и make.

```sh
make db-up                  # Postgres в Docker на localhost:5433
make run                    # бэкенд на :8080, миграции применяются при старте
npm --prefix frontend ci    # один раз
make web                    # фронт на http://localhost:5173
```

## Частые команды

| Команда | Что делает |
|---|---|
| `make gen` | перегенерировать код после правки `api/openapi.yaml` |
| `make check` | vet, тесты, проверка типов (то же, что в CI) |
| `make db-psql` | консоль psql в dev-базе |
| `make db-reset` | удалить dev-базу целиком |

Новая миграция: файл `backend/internal/db/migrations/NNNNN_name.sql`
с секциями `-- +goose Up` / `-- +goose Down`.

## Структура

```
api/openapi.yaml          контракт API
backend/
  cmd/server/             точка входа
  internal/api/           сгенерированный код (не править руками)
  internal/config/        настройки из env
  internal/db/            пул соединений, миграции
  internal/httpapi/       роутер, middleware, хендлеры
frontend/
  src/api/                типизированный клиент и сгенерированные типы
docs/                     архитектура и решения
```
