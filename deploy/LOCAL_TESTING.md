# dosedu.kz — Локалда Docker-мен тексеру

Бұл нұсқаулық **нақты домен мен SSL сертификатсыз**, тек өз
компьютеріңізде (localhost) бүкіл жүйені көтеріп көру үшін.

## 1. `/etc/hosts` файлына жазба қосыңыз

Субдомен-негізді роутинг жұмыс істеуі үшін, барлық `.dosedu.local`
атауларын `127.0.0.1`-ге бағыттау керек.

**macOS / Linux:**
```bash
sudo tee -a /etc/hosts << 'EOF'
127.0.0.1 dosedu.local
127.0.0.1 s-admin.dosedu.local
127.0.0.1 director.dosedu.local
127.0.0.1 teacher.dosedu.local
127.0.0.1 app.dosedu.local
EOF
```

**Windows** (Әкімші құқығымен PowerShell):
```powershell
Add-Content -Path C:\Windows\System32\drivers\etc\hosts -Value @"
127.0.0.1 dosedu.local
127.0.0.1 s-admin.dosedu.local
127.0.0.1 director.dosedu.local
127.0.0.1 teacher.dosedu.local
127.0.0.1 app.dosedu.local
"@
```

## 2. Бүкіл стекті бір командамен көтеру

Жоба түбірінде (`docker-compose.local.yml` жатқан жерде):

```bash
docker compose -f docker-compose.local.yml up --build
```

Бұл команда:
- **Go API**-ды құрастырады (`Dockerfile`)
- **Барлық 5 frontend қосымшаны** (`frontend/Dockerfile`) build қылып, соларды бір Nginx image-іне пісіреді
- **PostgreSQL**-ды көтеріп, `internal/db/migrations/001_init.sql`-ды автоматты орындайды
- **Redis**-ті көтереді
- **Nginx**-ты порт 80-де іске қосады (SSL жоқ, тек HTTP)

Бірінші рет көтергенде frontend build-і (npm install × 5) біраз уақыт
алуы мүмкін (~2–3 минут интернет жылдамдығына байланысты).

## 3. Ашып көру

| URL | Не көрсетеді |
|---|---|
| http://dosedu.local | Басты бет — лид формасы, кіру сілтемесі |
| http://s-admin.dosedu.local | Супер Админ кірісі (10 таңбалы пароль) |
| http://director.dosedu.local | Директор кірісі (8 таңбалы пароль) + Kanban CRM |
| http://teacher.dosedu.local | Мұғалім кірісі (6+ таңба) + тест жүктеу панелі |
| http://app.dosedu.local | Оқушы/ата-ана кірісі (4 цифрлы PIN) |

API тікелей де қолжетімді: `http://localhost:8080` (мысалы,
`curl http://localhost:8080/s-admin/health` — токенсіз 401 қайтарады,
бұл дұрыс жауап).

## 4. Кіру үшін тест деректерін жасау

Дерекқорда әзірше ешбір пайдаланушы жоқ (миграция тек схеманы
жасайды). Тест үшін тікелей PostgreSQL-ге қосылып, бір директор
жазбасын қолмен енгізуге болады:

```bash
docker compose -f docker-compose.local.yml exec postgres psql -U dosedu -d dosedu
```

```sql
-- алдымен бір филиал керек
INSERT INTO branches (name, address) VALUES ('Тест филиалы', 'Алматы') RETURNING id;
-- жоғарыдағы id-ды көшіріп, төменде branch_id ретінде қойыңыз

-- пароль хэшін bcrypt-пен алдын ала есептеу керек (мыс. https://bcrypt-generator.com
-- немесе `htpasswd -bnBC 10 "" 'Passw0rd!' | tr -d ':\n' | sed 's/^\$2y/\$2a/'`)
INSERT INTO directors (branch_id, email, password_hash, full_name)
VALUES ('<branch_id>', 'director@test.kz', '<bcrypt_hash>', 'Тест Директор');
```

Содан кейін http://director.dosedu.local бетінде осы email/парольмен
кіруге болады.

## 5. Тоқтату / тазалау

```bash
docker compose -f docker-compose.local.yml down          # тоқтату
docker compose -f docker-compose.local.yml down -v        # + дерекқор көлемін өшіру (толық тазалау)
```

## Ескертулер

- Бұл конфигурация **тек локал тестілеу үшін** — SSL жоқ, құпия
  сөздер (`JWT_SECRET`, PostgreSQL parolь) хардкодталған, production-да
  ешқашан қолданбаңыз.
- Telegram Bot интеграциясы токенсіз "жұмыс істейді" (яғни хабарлама
  жібермей, тек no-op қайтарады) — нақты хабарлама сынау үшін
  `docker-compose.local.yml`-дегі `TELEGRAM_BOT_TOKEN`-ды толтырып,
  жергілікті ортаны сыртқа шығаратын tunnel (мыс. `ngrok`) арқылы
  webhook тіркеу керек.
- `nginx` контейнерінің build уақыты ұзақ көрінсе — бұл 5 Vue
  қосымшаны бір-бірлеп `npm install` жасайтындықтан қалыпты жағдай.
