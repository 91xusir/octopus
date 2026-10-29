import { clsx, type ClassValue } from "clsx"
import { twMerge } from "tailwind-merge"
import { displayUSD, useSettingStore } from "@/stores/setting";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}


function formatNumber(num: number | undefined, compare: number[], units: string[]): { value: string, unit: string } {
  if (num === undefined) return { value: "0.00", unit: units[0] };
  else if (num >= compare[0]) return { value: (num / compare[0]).toFixed(2), unit: units[1] };
  else if (num >= compare[1]) return { value: (num / compare[1]).toFixed(2), unit: units[2] };
  else if (num >= compare[2]) return { value: (num / compare[2]).toFixed(2), unit: units[3] };
  else if (num >= compare[3]) return { value: (num / compare[3]).toFixed(2), unit: units[4] };
  else return { value: (num).toFixed(2), unit: units[5] };
}

export function formatCount(num: number | undefined): { raw: number, formatted: { value: string, unit: string } } {
  return {
    raw: num ?? 0,
    formatted: formatNumber(num, [1000000000, 1000000, 1000, 1], ['', 'B', 'M', 'K', '', '']),
  };
}
// formatMoney 按显示货币格式化美元金额: raw 恒为美元原值(排序与配额计算依赖), formatted 为当前货币的展示值。
// 人民币只是显示层, 换算发生在读 store 的这一刻; 切换货币后由调用方失效查询缓存触发重算。
export function formatMoney(num: number | undefined): { raw: number, formatted: { value: string, unit: string } } {
  if (useSettingStore.getState().displayCurrency === 'CNY') {
    return {
      raw: num ?? 0,
      formatted: formatNumber(displayUSD(num ?? 0), [1000000000, 1000000, 1000, 1], ['¥', 'B¥', 'M¥', 'K¥', '¥', '¥']),
    };
  }
  return {
    raw: num ?? 0,
    formatted: formatNumber(num, [1000000000, 1000000, 1000, 1], ['$', 'B$', 'M$', 'K$', '$', '$']),
  };
}

// formatModelPrice 格式化每百万 token 的模型单价; 美元沿用原有的后置 $ 写法, 人民币改为前置 ¥。
export function formatModelPrice(usd: number): string {
  if (useSettingStore.getState().displayCurrency === 'CNY') {
    return `¥${displayUSD(usd).toFixed(2)}`;
  }
  return `${usd.toFixed(2)}$`;
}

export function formatTime(ms: number | undefined): { raw: number, formatted: { value: string, unit: string } } {
  return {
    raw: ms ?? 0,
    formatted: formatNumber(ms, [86400000, 3600000, 60000, 1000], ['', 'd', 'h', 'm', 's', 'ms']),
  };
}