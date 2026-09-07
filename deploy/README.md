# dosedu.kz — Deployment (Docker Compose + Nginx + Wildcard SSL)

## Құрылым

```
deploy/
├── nginx/
│   ├── conf.d/
│   │   ├── 00-http.conf        # HTTP -> HTTPS redirect + ACME challenge path
│   │   ├── 01-upstream.conf    # upstream + rate-limit zones (http context)
│   │   ├── 10-dosedu-kz.conf   # public landing + lead form + login
│   │   ├── 20-s-admin.conf     # super admin (+ optional IP allowlist)
│   │   ├── 30-director.conf    # director cabinet
│   │   ├── 40-teacher.conf     # teacher cabinet (larger upload limit)
│   │   └── 50-app.conf         # student/parent portal
│   └── snippets/
│       ├── ssl-params.conf     # shared TLS hardening
│       └── proxy-params.conf   # shared reverse-proxy headers
└── certbot/                    # cert storage (mounted into nginx + certbot)
```

## Неге wildcard SSL DNS-01 арқылы алынады

Cпецификацияда "Wildcard SSL" деп көрсетілген — яғни бір сертификат
`*.dosedu.kz` барлық субдоменді (`s-admin.`, `director.`, `teacher.`,
`app.`) жабады. Let's Encrypt wildcard сертификатты **тек DNS-01
challenge арқылы** береді (HTTP-01 жұмыс істемейді). Сондықтан бастапқы
шығару қолмен, домен провайдеріңіздің DNS API-і арқылы жасалады.

### 1. Бастапқы сертификат шығару (бір рет, қолмен)

```bash
docker run -it --rm \
  -v "$(pwd)/deploy/certbot/conf:/etc/letsencrypt" \
  -v "$(pwd)/deploy/certbot/www:/var/www/certbot" \
  certbot/certbot certonly \
  --manual --preferred-challenges dns \
  -d "dosedu.kz" -d "*.dosedu.kz" \
  --email admin@dosedu.kz --agree-tos --no-eff-email
```

Certbot TXT жазба мәнін көрсетеді — оны домен провайдеріңіздің DNS
басқармасында `_acme-challenge.dosedu.kz` үшін қосыңыз, содан кейін
жалғастырыңыз. Егер провайдеріңіздің Certbot DNS плагині бар болса
(мысалы Cloudflare, Route53), `--manual`-дың орнына сол плагинді
пайдаланып, автоматтандыруға болады — бұл жаңарту скриптін де оңайтады.

### 2. Автоматты жаңарту

`docker-compose.yml`-дегі `certbot` қызметі әр 12 сағат сайын
`certbot renew` жүргізеді. Бірақ wildcard сертификат **DNS-01**-мен
шығарылғандықтан, `renew` де DNS плагинсіз автоматты жұмыс істемейді —
DNS плагинді (`certbot-dns-cloudflare` және т.б.) қолданбасаңыз, әр
~80 күн сайын 1-қадамды қолмен қайталау керек болады.

## Іске қосу

```bash
cp .env.example .env
# .env файлындағы құпия мәндерді толтырыңыз

# 1) алдымен сертификатты жоғарыдағы қадаммен шығарыңыз
# 2) содан кейін бүкіл стекті көтеріңіз
docker compose up -d --build

# логтарды бақылау
docker compose logs -f api
```

## Дерекқор миграциясы

`postgres` контейнері бірінші рет көтерілгенде
`internal/db/migrations/001_init.sql` файлын автоматты орындайды
(`docker-entrypoint-initdb.d` арқылы). Кейінгі миграцияларды қолмен
қосу үшін:

```bash
docker compose exec -T postgres psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" < internal/db/migrations/002_xxx.sql
```

## Telegram Bot webhook тіркеу

API көтерілгеннен және `dosedu.kz` HTTPS-пен қолжетімді болғаннан кейін,
Telegram-ды `/public/telegram/webhook`-ке хабарлама жіберуге бір рет
тіркеу керек:

```bash
curl -X POST "https://api.telegram.org/bot${TELEGRAM_BOT_TOKEN}/setWebhook" \
  -d "url=https://dosedu.kz/api/public/telegram/webhook" \
  -d "secret_token=${TELEGRAM_WEBHOOK_SECRET}"
```

`TELEGRAM_WEBHOOK_SECRET` мәні `.env`-дегімен бірдей болуы керек — API
осы мәнді әрбір келген сұраныстың `X-Telegram-Bot-Api-Secret-Token`
тақырыбымен салыстырып, шынымен Telegram-нан келгенін растайды.

## Frontend орналастыру ескертуі

Nginx конфигурациясындағы `root /usr/share/nginx/html/<subdomain>`
жолдары — әр Vue/Nuxt frontend жобасының build өнімі сол бумаларға
көшірілуі керек (немесе жеке Docker image/volume ретінде маунт етіледі).
Бұл skeleton-да frontend жобалары қамтылмаған.
