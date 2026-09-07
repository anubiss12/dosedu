# dosedu.kz — Frontend Workspace (Vue 3 + Vite + vue-i18n)

Бес бөлек Vue 3 қосымшадан тұратын monorepo — әрқайсысы бір субдоменге
сәйкес келеді, техникалық тапсырмадағы тіл конфигурациясымен:

| Бума | Субдомен | Тілдер |
|---|---|---|
| `landing` | `dosedu.kz` | Қазақша, Русский, English, 中文 |
| `s-admin` | `s-admin.dosedu.kz` | Қазақша, English |
| `director` | `director.dosedu.kz` | Қазақша, Русский |
| `teacher` | `teacher.dosedu.kz` | Қазақша, Русский, English, 中文 |
| `app` | `app.dosedu.kz` (оқушы/ата-ана) | Қазақша, Русский, English, 中文 |

Әрқайсысында бірдей құрылым бар:

```
<package>/
├── index.html
├── package.json
├── vite.config.ts
└── src/
    ├── main.ts
    ├── App.vue
    ├── i18n.ts                 # vue-i18n баптауы, сол домен тілдерімен ғана
    ├── locales/<lang>.json      # аудармалар
    ├── composables/
    │   └── useTheme.ts          # Dark/Light режим (барлық субдомендерде ортақ талап)
    ├── components/
    │   ├── LangSwitcher.vue
    │   └── ThemeToggle.vue
    └── views/                   # сол кабинеттің беттері
```

## Іске қосу

```bash
pnpm install
pnpm --filter landing dev     # dosedu.kz үлгісі, http://localhost:5173
pnpm --filter s-admin dev
pnpm --filter director dev
pnpm --filter teacher dev
pnpm --filter app dev
```

Production build әрқайсысын жеке `dist/` бумасына шығарады — сол
бумаларды backend деплой нұсқаулығындағы (`deploy/README.md`) Nginx
`root` жолдарына (`/usr/share/nginx/html/<subdomain>`) көшіру керек.

## Dark/Light режим

`useTheme.ts` composable-і барлық 5 қосымшада ортақ өрнек: `<html>`
элементіне `data-theme="dark"|"light"` атрибутын қойып, CSS
айнымалылары арқылы түстерді ауыстырады, таңдауды `localStorage`-те
сақтайды. `ThemeToggle.vue` компоненті жоғарғы оң жақ бұрышта
орналасады (спецификацияға сай).

## i18n құрылымы

Әр `locales/<lang>.json` файлы бірдей кілт құрылымын ұстанады
(`nav.*`, `auth.*`, беттерге тән бөлімдер), сондықтан жаңа тіл қосу —
жаңа JSON файл жазып, `i18n.ts`-тегі `messages` тізіміне қосу ғана.
Ортақ кілттерді (`common.json`) бөлек шығарып, әр домен өз locale
файлында импорттауға болады — қазірше қарапайымдылық үшін әр тіл бір
файлда.
