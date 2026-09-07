# Логотипті ауыстыру

Әр frontend қосымшада (`landing`, `s-admin`, `director`, `teacher`,
`app`) уақытша **placeholder** логотип тұр: `public/logo.svg`
(көк дөңгелектелген шаршы ішінде "DE" әріптері).

## Нақты логотипті қою

Нақты лого дайын болғанда, оны сол файлдардың орнына көшіріп қойыңыз:

```
frontend/landing/public/logo.svg
frontend/s-admin/public/logo.svg
frontend/director/public/logo.svg
frontend/teacher/public/logo.svg
frontend/app/public/logo.svg
```

**Файл атауы дәл `logo.svg` болуы керек** (немесе кодтағы
`<img src="/logo.svg">` жолын да сәйкес өзгерту керек). Файл форматы
SVG болса ең жақсы (кез келген өлшемде анық көрінеді); PNG/JPG да
жұмыс істейді, бірақ сол жағдайда файл атауын да, кодтағы
`src="/logo.svg"` жолын да (мыс. `src="/logo.png"`) сәйкес өзгерту
керек.

Логотип өлшемі кодта автоматты түрде 28×28px-ке дейін кішірейтіледі
(`.logo` CSS класы, `frontend/*/src/style.css` файлында). Егер өлшемді
өзгерткіңіз келсе, сол жердегі `width`/`height` мәндерін түзетіңіз.

## Өзгертуден кейін

Логотипті ауыстырғаннан кейін, frontend image-ін қайта build қылу
керек (себебі static файлдар Docker image-іне build кезінде
"пісіріледі"):

```powershell
docker compose -f docker-compose.local.yml up --build
```
