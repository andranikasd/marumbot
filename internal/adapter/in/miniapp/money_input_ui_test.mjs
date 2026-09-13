import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import vm from 'node:vm';
const source=await readFile(new URL('./web/js/core.js',import.meta.url),'utf8');
const parser=source.slice(source.indexOf('export const moneyNum'),source.indexOf('export const esc')).replace('export const','const');
const env={};vm.createContext(env);vm.runInContext(parser+'\nglobalThis.parse=moneyNum;',env);
for(const [input,want] of [['1000',1000],['1 000',1000],['1,000',1000],['1.000',1000],['1,000.50',1000.5],['1.000,50',1000.5],['1000,50',1000.5],['1,000,000',1000000],['0',0],['0.25',0.25],['0.01',0.01]])assert.equal(env.parse(input),want,input);
for(const input of ['0.001','0,001','0,001.00','0,001,000','1,2.34','1.2,34','1,2345','1.2345','1,23,456','1.23.456','1,000.','1,000.123','1,2,3','1000,000','1,000,','1,000.2.3','-5','NaN','Infinity',''])assert.ok(Number.isNaN(env.parse(input)),input+' must not silently become a different amount');
console.log('Money input accepts supported separators and rejects malformed grouping without changing amounts.');
