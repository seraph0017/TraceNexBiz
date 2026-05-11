// 共享 i18n bundle 入口（W0 scaffold）。
// W1e/f/g：按 frontend §10 拆 namespace（common / customer / partner / admin / errors）.
import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';
import zhCN from './locales/zh-CN/common.json';
import enUS from './locales/en-US/common.json';

export async function initI18n(defaultLng: 'zh-CN' | 'en-US' = 'zh-CN'): Promise<typeof i18n> {
  if (!i18n.isInitialized) {
    await i18n.use(initReactI18next).init({
      lng: defaultLng,
      fallbackLng: 'zh-CN',
      ns: ['common'],
      defaultNS: 'common',
      resources: {
        'zh-CN': { common: zhCN },
        'en-US': { common: enUS },
      },
      interpolation: { escapeValue: false },
    });
  }
  return i18n;
}

export { default as i18n } from 'i18next';
