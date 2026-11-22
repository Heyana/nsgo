package main

import "testing"

func TestMiniProgramHostSignIn(t *testing.T) {
	vm := runMiniProgramModule(t, "signin.js")
	_, err := vm.RunString(`
const login = module.exports;
function check(condition,message) {if(!condition)throw new Error(message)}
for (const platform of ['wechat','qq','douyin','alipay']) {
  let calls=0, result, prompt;
  const api = platform==='alipay' ? {getAuthCode(o) {check(o.scopes.join()==='auth_base','extra scopes');o.success({authCode:'owned'});o.fail({});}}
    : {login(o) {prompt=o.force;check(this===api,'lost receiver');o.success({code:'owned',isLogin:true});o.fail({});}};
  check(login.available(platform,api),'host capability missing');
  login.signIn(platform,api,{prompt:true,complete(r){calls++;result=r}});
  check(calls===1 && result.platform===platform && result.code==='owned' && !result.error,'incorrect host answer');
  if(platform==='douyin')check(prompt===true,'lost prompt');
}
let result;
const complete=r=>result=r;
login.signIn('douyin',{login(o){check(o.force===false,'implicit host prompt');o.success({anonymousCode:'device-only',isLogin:false})}},{complete});
check(result.error==='notSignedIn' && result.code==='','anonymous device signed in');
login.signIn('douyin',{login(o){o.fail({errorCode:113001,errMsg:'login:fail host login fail'})}},{complete});
check(result.error==='cancelled','lost cancellation');
for(const platform of ['wechat','qq','douyin','alipay']) {
  login.signIn(platform,{}, {complete});check(result.error==='unavailable','missing API');
  const method=platform==='alipay'?'getAuthCode':'login';
  for(const value of [null,{}, {code:42}, {code:'',anonymousCode:'device-only'}]) {
    login.signIn(platform,{[method](o){o.success(value)}},{complete});
    check(result.error==='failed' && result.code==='','invalid code accepted');
  }
  login.signIn(platform,{[method](){throw new Error('owned refusal')}},{complete});
  check(result.error==='failed','exception escaped');
}
let later,calls=0;
login.signIn('wechat',{login(o){later=o}},{complete(r){calls++;result=r}});
check(calls===0,'async completed early');
later.success({code:'owned'});later.success({code:'duplicate'});later.fail({});
check(calls===1 && result.code==='owned','duplicate completion');
login.signIn('unknown',{login(){}},{complete});check(result.error==='unavailable','unknown host accepted');
`)
	if err != nil {
		t.Fatal(err)
	}
}
