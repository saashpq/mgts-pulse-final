/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_API_URL?: string;
  /** 'true' — статическое онлайн-демо: данные из /demo/*.json, запись запрещена */
  readonly VITE_STATIC_DEMO?: string;
}
