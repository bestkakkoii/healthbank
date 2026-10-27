// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import assert from 'node:assert/strict';
import test from 'node:test';
import {exactDifference,recordDifference,latestDistinctPair,nonHdlCholesterol} from '../src/analysis-summary.ts';
function row(patch={}){return {item_id:'test',value_kind:'numeric',result:'1.2',numeric_value:1.2,unit:'mg/dL',exam_date:'2026-10-27',visit_date:'2026-10-27',institution:'SYNTHETIC',...patch};}
test('decimal differences retain exactness and safe percentage conditions',()=>{
 assert.equal(exactDifference('0.3','0.2'),'+0.1');assert.equal(exactDifference('1e2','90'),'+10');assert.equal(exactDifference('-2','-3'),'+1');assert.equal(exactDifference('0','0'),'0');assert.equal(exactDifference('>2','1'),null);
 const diff=recordDifference(row(),row({result:'1.1',numeric_value:1.1,exam_date:'2026-10-20'}));assert.equal(diff.difference,'+0.1');assert.equal(diff.days,7);
 assert.equal(recordDifference(row(),row({result:'0',numeric_value:0})).percent,null);
 assert.equal(recordDifference(row(),row({numeric_value:99})),null);
 assert.equal(recordDifference(row(),row({unit:'mmol/L'})),null);
 assert.deepEqual(latestDistinctPair([row(),row(),row({exam_date:'2026-01-01'})])[0],undefined);
});
test('non-HDL refuses implicit pairing and never substitutes missing values',()=>{
 const tc=row({order_code:'09001C',result:'200.1',numeric_value:200.1});const hdl=row({order_code:'09043C',result:'40',numeric_value:40});
 assert.equal(nonHdlCholesterol(tc,hdl,true).value,'160.1');assert.equal(nonHdlCholesterol(tc,hdl,false).value,null);
 for(const patch of [{exam_date:'2026-10-26'},{institution:'OTHER'},{visit_date:''},{unit:'mmol/L'},{value_kind:'gte'},{result:'400',numeric_value:400}])assert.equal(nonHdlCholesterol(tc,{...hdl,...patch},true).value,null);
});
