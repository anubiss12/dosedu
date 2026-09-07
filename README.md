# dosedu.kz — Backend Skeleton (Go / Gin)

Техникалық тапсырмаға сай құрылған бастапқы қаңқа. Бұл толық өнім емес —
негізгі архитектуралық қаңқа: дерекқор схемасы, авторизация/пароль
саясаттары, сессия менеджменті, роутинг және басты handler-лердің
интерфейстері.

## Құрылым

```
dosedu/
├── cmd/api/main.go              # entrypoint: config, postgres, redis, router
├── internal/
│   ├── config/                  # env-based configuration
│   ├── auth/
│   │   ├── password.go          # рөл бойынша пароль саясаты + bcrypt
│   │   └── session.go           # JWT + Redis idle-timeout (30 мин)
│   ├── middleware/auth.go       # Gin middleware: token тексеру, рөл шектеу
│   ├── repository/              # pgx негізді дерекқор қабаты
│   │   ├── store.go             #   ортақ pgx pool
│   │   ├── credentials.go       #   логин іздеу (рөл бойынша)
│   │   ├── leads.go             #   CRM / Kanban
│   │   ├── students.go          #   прогресс, баланс, ConfirmPayment
│   │   ├── schedule.go          #   кесте + бөлме қақтығысын аудару
│   │   ├── parents.go           #   Telegram chat байланысы, /balance
│   │   └── attendance.go        #   QR check-in, айлық статистика
│   ├── telegram/                # Telegram Bot интеграциясы
│   │   ├── client.go            #   sendMessage/sendDocument/setWebhook
│   │   ├── notifications.go     #   QR/төлем/баланс хабарлама үлгілері
│   │   └── webhook.go           #   /start <телефон>, /balance командалары
│   ├── pdfreport/generate.go    # тәуелсіз (сыртқы кітапхана жоқ) PDF генератор
│   ├── handlers/
│   │   ├── router.go            # субдомен бойынша маршруттар (Deps әдістері)
│   │   ├── leads.go             # CRM handler-лері
│   │   ├── test_upload.go       # деңгейлік тест жүктеу + error log
│   │   ├── attendance_and_telegram.go  # QR check-in, telegram webhook
│   │   ├── telegram_adapter.go  # repository <-> telegram интерфейс адаптері
│   │   ├── pdf_report.go        # айлық PDF есеп (жүктеу / Telegram-мен жіберу)
│   │   └── auth_and_stubs.go    # Deps struct, login, қалған endpoint-тер
│   └── db/migrations/001_init.sql  # толық PostgreSQL схемасы
├── deploy/
│   ├── nginx/conf.d/            # субдомен бойынша server block-тар (production, SSL)
│   ├── nginx/local/             # локал тестілеу үшін HTTP-only конфигурация
│   ├── nginx/snippets/          # ортақ SSL/proxy параметрлері
│   ├── certbot/                 # сертификат сақтау орны (volume)
│   ├── README.md                # wildcard SSL + Telegram webhook тіркеу нұсқаулығы
│   └── LOCAL_TESTING.md         # Docker-мен localhost-та тексеру нұсқаулығы
├── frontend/                    # 5 Vue 3 + i18n қосымша (landing/s-admin/director/teacher/app)
│   └── Dockerfile               # барлық 5-еуін бір Nginx image-іне build қылады
├── docker-compose.yml           # production: api + postgres + redis + nginx + certbot
├── docker-compose.local.yml     # localhost тестілеу: SSL-сіз, бір командамен
├── Dockerfile                   # Go API multi-stage build
├── .dockerignore
├── .env.example                 # құпия айнымалылар үлгісі
└── scripts/backup.sh            # түнгі pg_dump cron скрипті
```

## Тексерілген (verified) бөліктер

- **`go build ./...`** және **`go vet ./...`** — толық компиляцияланды, ескертусіз (exit 0)
- **`nginx -t`** — барлық 7 server block (HTTP redirect + dosedu.kz + s-admin + director + teacher + app) синтаксисі расталды
- **`docker-compose.yml`** — жарамды YAML

> **Ескерту `go.sum` туралы:** `go.sum` файлы репозиторийден
> әдейі алынып тасталды — `Dockerfile`-дағы `go mod tidy` қадамы
> оны Docker build кезінде (қалыпты интернеті бар ортада) қайта
> жасайды. Егер жергілікті түрде `go build`/`go run` жасағыңыз келсе,
> алдымен `go mod tidy` жүргізіңіз.

> **Ескерту PDF генератор туралы:** `internal/pdfreport` сыртқы
> тәуелділіксіз, қолмен жазылған PDF генератор (base-14 Helvetica
> қарпімен). Бұл тек Latin таңбаларды қолдайды — кирилл есімдер `?`
> болып шығады. Production-ға шығармас бұрын `go-pdf/fpdf` немесе
> ұқсас кітапхананы кириллицаны қолдайтын TrueType қарыппен
> (мыс. DejaVuSans.ttf) ауыстырыңыз. `Generate()` функциясының
> сигнатурасы солай өзгермейтіндей етіп жобаланған.

## Спецификациямен сәйкестік

| Талап | Іске асыру |
|---|---|
| Рөл бойынша пароль саясаты (4/6/8/10 таңба) | `internal/auth/password.go` — regex-негізді валидация, содан кейін ғана bcrypt хештеу |
| 30 мин әрекетсіздіктен logout (staff рөлдер) | `internal/auth/session.go` — Redis TTL кілті, `TouchSession` әр сұраныста жаңартады |
| Субдомендер бойынша қатынас | `internal/handlers/router.go` — `s-admin`, `director`, `teacher`, `family` route group-тары, әрқайсысы өз рөліне шектелген middleware-мен |
| Тест жүктеу — өлшем/формат тексеру + error log | `internal/handlers/test_upload.go` — 5 МБ лимит, .csv/.xlsx тексеру, жол-жол қате жинау |
| Leads CRM / Kanban | `internal/handlers/leads.go` — public lead формасы + директордың stage ауыстыру endpoint-і |
| Кабинет-бөлек аймақ (branch scoping) | Барлық directors/teachers/students кестелерінде `branch_id`, JWT claims-те де бар |
| Бөлме/топ қақтығысын болдырмау | `schedule_slots` кестесіндегі PostgreSQL `EXCLUDE` шектеуі — бір бөлме бір уақытта екі рет броньланбайды |
| Автоматты сақтық көшірме | `scripts/backup.sh` — cron арқылы түнгі `pg_dump`, ескі файлдарды тазалау |
| CRM→LMS авто-көшу | `internal/handlers/leads.go` — лид "Төлем жасады" бағанына ауысқанда, `provisionStudentFromLead` арқылы оқушы+логин авто-генерацияланады |
| Сертификат генераторы + QR | `internal/certificate/` — hand-rolled PDF, QR коды вектор ретінде тікелей салынады (сурет ендіру қажет емес) |
| Internal Ticketing | `internal/repository/tickets.go` + `internal/handlers/tickets.go` — мұғалім/директор/ата-ана арасындағы тред |
| Кеңейтілген деңгейлер | Teacher кабинетінде CEFR (A1–C1) + HSK (1–6) толық тізімі |
| Tailwind CSS, responsive | Барлық 5 frontend қосымшада (`sm/md/lg/xl` breakpoint-тер) |
| Swagger / OpenAPI | `docs/` (Docker build кезінде `swag init` арқылы автогенерацияланады) — `/swagger/index.html` |

## Әрі қарай не істеу керек

1. i18n: `dosedu.kz` мен `teacher.` үшін 4 тіл, `director.` мен `s-admin.`
   үшін 2 тіл — frontend (Vue/Nuxt) жағында `vue-i18n` арқылы.
2. Frontend (Vue.js/Nuxt.js + Tailwind) жобалары — әр субдоменге бөлек,
   `deploy/nginx/conf.d/*.conf`-тағы `root` жолдарына сай build шығысымен.
3. `CreateBranch`, `CreateDirector`, `CreateTeacher`, `MarkAttendance`,
   `UploadLevelTest`-тің дерекқорға сақтау бөлігі — қазір JSON жауап
   қайтарады, бірақ `repository` қабатына жазу шақыруы толтырылмаған.
4. PDF генераторды кириллицаны қолдайтын кітапханаға ауыстыру (жоғарыдағы
   ескертуді қараңыз).
5. Толық интеграциялық тест жиынтығы.

## Іске қосу (жергілікті, Docker-сіз)

```bash
export POSTGRES_DSN="postgres://dosedu:dosedu@localhost:5432/dosedu?sslmode=disable"
export REDIS_ADDR="localhost:6379"
export JWT_SECRET="replace-with-strong-secret"

psql "$POSTGRES_DSN" -f internal/db/migrations/001_init.sql
psql "$POSTGRES_DSN" -f internal/db/migrations/002_certificates_tickets.sql

# main.go blank-imports the generated docs/ package (Swagger UI), so
# it must exist before `go run`/`go build` work locally. Docker does
# this automatically (see Dockerfile) — for local dev, run it once
# and re-run whenever you add/change @-annotated handler comments:
go install github.com/swaggo/swag/cmd/swag@v1.16.3
swag init -g cmd/api/main.go -o ./docs

go run ./cmd/api
```

## API құжаттамасы (Swagger / OpenAPI)

Сервер көтерілгеннен кейін интерактивті құжаттама:
**http://localhost:8080/swagger/index.html**

Жаңа endpoint қосқанда немесе бар handler-ді өзгерткенде, оның үстіне
swaggo пішіміндегі комментарий қосыңыз (`@Summary`, `@Tags`, `@Param`,
`@Success` т.б. — мысалдары: `internal/handlers/auth_and_stubs.go`-дағы
`Login`, `internal/handlers/leads.go`-дағы `MoveLeadStage`). Docker
build кезінде `swag init` автоматты түрде осы комментарийлерден
`docs/`-ты қайта генерациялайды — қолмен қайталап жүргізудің қажеті
жоқ (Docker үшін). Жергілікті (Docker-сіз) дамытуда әр өзгерістен
кейін жоғарыдағы `swag init` командасын қайта жүргізіңіз.

## Іске қосу (Docker Compose — толық стек)

Толық нұсқаулық: [`deploy/README.md`](deploy/README.md) (wildcard SSL
шығару, DNS-01 challenge, .env баптау).

```bash
cp .env.example .env   # құпия мәндерді толтырыңыз
docker compose up -d --build
```

## Локалда, SSL-сіз тексеру (Docker қана)

Нақты домен/сертификатсыз, тек өз компьютеріңізде толық стекті
(backend + барлық 5 frontend + PostgreSQL + Redis + Nginx) бір
командамен көтеру үшін: [`deploy/LOCAL_TESTING.md`](deploy/LOCAL_TESTING.md).

```bash
docker compose -f docker-compose.local.yml up --build
```
