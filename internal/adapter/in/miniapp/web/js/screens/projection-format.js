"use strict";
// Money travels to the API as integer minor units. Parse the text exactly;
// never round an over-precise input or pass an unsafe integer to JSON.
export function extraMinor(text,exponent){
 if(!Number.isInteger(exponent)||exponent<0||exponent>3)return null;
 const value=String(text).trim().replace(/[\s\u00a0]/g,'').replace(',','.');
 if(!/^\d+(\.\d+)?$/.test(value))return null;
 const [whole,fraction='']=value.split('.');
 if(fraction.length>exponent)return null;
 const minor=BigInt(whole)*10n**BigInt(exponent)+BigInt((fraction+'0'.repeat(exponent)).slice(0,exponent)||'0');
 return minor<=BigInt(Number.MAX_SAFE_INTEGER)?Number(minor):null;
}
export function extraText(minor,exponent){
 if(!Number.isSafeInteger(minor)||minor<0)throw new Error('Invalid amount');
 const scale=10n**BigInt(exponent),n=BigInt(minor);
 return exponent?`${n/scale}.${String(n%scale).padStart(exponent,'0')}`:String(n);
}
export function validProjection(value){
 return value&&/^\d{4}-\d{2}-\d{2}$/.test(value.today)&&Array.isArray(value.currencies)&&value.currencies.every(c=>
 /^[A-Z]{3}$/.test(c.currency)&&Number.isInteger(c.exponent)&&c.exponent>=0&&c.exponent<=3&&Array.isArray(c.months)&&c.months.every(m=>
 /^\d{4}-(0[1-9]|1[0-2])$/.test(m.month)&&['required_minor','extra_minor','total_minor'].every(k=>Number.isSafeInteger(m[k])&&m[k]>=0)&&Array.isArray(m.loans)&&m.loans.every(l=>typeof l.id==='string'&&typeof l.name==='string'&&['required_minor','extra_minor'].every(k=>Number.isSafeInteger(l[k])&&l[k]>=0))));
}
