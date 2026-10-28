// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import assert from 'node:assert/strict';
import test from 'node:test';
import {parseIntakeRecords,decodeIntakeFile,intakeStorageKey,intakeOnDate} from '../src/personal-records.ts';
import {portableReportJSON,portableReportXML} from '../src/portable-report.ts';
const row={id:'00000000-0000-0000-0000-000000000001',name:'SYNTHETIC',category:'other',amount:'',start:'2026-01-01',end:'2026-01-31',note:''};
test('optional intake records remain scoped and reject unexpected sensitive fields',()=>{
 assert.equal(intakeStorageKey(''),null);assert.equal(intakeStorageKey('A123456789'),null);
 assert.equal(intakeStorageKey('a'.repeat(64)),`healthbank.intake.v1.${'a'.repeat(64)}`);
 assert.deepEqual(parseIntakeRecords(JSON.stringify([row])),[row]);
 for(const bad of [[{...row,password:'must reject'}],[row,row],[{...row,start:'2026-02-30'}],[{...row,start:'2026-02-01'}],[{...row,name:' '}],Array(201).fill(row)])assert.throws(()=>parseIntakeRecords(JSON.stringify(bad)));
 assert.equal(intakeOnDate([row],'2026-01-31').length,1);assert.equal(intakeOnDate([row],'2026-02-01').length,0);assert.equal(intakeOnDate([{...row,start:''}],'2026-01-01').length,0);
});
test('personal note imports reject duplicate and escaped-equivalent fields without choosing a value silently',()=>{
 const source=JSON.stringify([row]);
 for(const duplicate of ['"name":"changed","name":','"na\\u006de":"changed","name":'])
  assert.throws(()=>parseIntakeRecords(source.replace('"name":',duplicate)));
 assert.throws(()=>parseIntakeRecords(source.replace('"note":""','"note":"\\ud800"')));
 const unicode={...row,note:'合成中文😀'};
 assert.deepEqual(parseIntakeRecords(JSON.stringify([unicode])),[unicode]);
});
test('personal note files validate original UTF-8 bytes including BOM and reject replacement decoding',()=>{
 const encoded=new TextEncoder().encode('\ufeff'+JSON.stringify([row]));
 assert.deepEqual(decodeIntakeFile(encoded),[row]);
 assert.throws(()=>decodeIntakeFile(new Uint8Array([0x5b,0xff,0x5d])),/UTF-8/);
 assert.throws(()=>decodeIntakeFile(new Uint8Array(768_001)),/過大/);
});
test('portable exports preserve raw rows and exclude inferred and identity fields',()=>{
 const raw={exam_date:'2026-10-01',visit_date:'2026-09-30',order_code:'SYNTHETIC',order_name:'fixture',item_name:'A&B',result:'<3',unit:'test',reference_text:'[1][2]',institution:'SYNTHETIC',source:'fixture',official_flag:'H',password:'never',numeric_value:99,level:1};
 const json=portableReportJSON([raw,raw]);const value=JSON.parse(json);assert.equal(value.records.length,2);assert.equal(value.records[0].result,'<3');assert.equal(json.includes('password'),false);assert.equal(json.includes('numeric_value'),false);
 const xml=portableReportXML([raw]);assert.match(xml,/<item_name>A&amp;B<\/item_name>/);assert.match(xml,/<result>&lt;3<\/result>/);assert.throws(()=>portableReportXML([{...raw,result:'\u0001'}]));
 assert.match(portableReportJSON([{...raw,result:'\u0001'}]),/\\u0001/);
});
