import { create } from 'zustand';
import { persist } from 'zustand/middleware';

export type Locale = 'zh_hans' | 'zh_hant' | 'en';

// 金额的显示货币; 数据一律以美元存储, 人民币只是显示层的换算结果。
export type DisplayCurrency = 'USD' | 'CNY';

// 汇率的模块级快照: 由根组件从后端设置同步, 供格式化函数在 React 之外读取。
export const DEFAULT_USD_CNY_RATE = 7.2;
let usdCnyRate = DEFAULT_USD_CNY_RATE;

export function getUSDCNYRate(): number {
    return usdCnyRate;
}

export function setUSDCNYRate(rate: number) {
    usdCnyRate = rate;
}

// 美元金额换算为人民币显示值; USD 模式下原样返回。
export function displayUSD(usd: number): number {
    return useSettingStore.getState().displayCurrency === 'CNY' ? usd * usdCnyRate : usd;
}

interface SettingState {
    locale: Locale;
    setLocale: (locale: Locale) => void;
    displayCurrency: DisplayCurrency;
    setDisplayCurrency: (currency: DisplayCurrency) => void;
}

export const useSettingStore = create<SettingState>()(
    persist(
        (set) => ({
            locale: 'zh_hans',
            setLocale: (locale) => set({ locale }),
            displayCurrency: 'USD',
            setDisplayCurrency: (displayCurrency) => set({ displayCurrency }),
        }),
        {
            name: 'octopus-settings',
        }
    )
);

// 订阅显示货币的组件用这个取值, 货币切换时触发重渲染。
export const useDisplayCurrency = () => useSettingStore((state) => state.displayCurrency);
