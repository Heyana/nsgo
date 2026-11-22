package tsx_test

import "testing"

func TestAppURLServiceTracksAddressAndRejectsInvalidHistory(t *testing.T) {
	app := run(t, `import {mount,UrlScheme,Text,View,signal} from "nsgo";
 function App(){
  const result=signal("waiting");
  return <View><Text testID="address">{()=>UrlScheme.url()??"none"}</Text>
   <Text testID="set" onPress={()=>UrlScheme.setUrl("demo://selected",{history:"push"})}>Set</Text>
   <Text testID="invalid" onPress={()=>{try{UrlScheme.setUrl("demo://bad",{history:"other"})}catch(e){result.set(e.name)}}}>Invalid</Text>
   <Text>{()=>result()}</Text></View>
 }mount(App);`)
	app.Find("text:none")
	app.Press("id:set")
	app.Find("text:demo://selected")
	app.Press("id:invalid")
	app.Find("text:TypeError")
	app.Find("text:demo://selected")
	app.OpenLinks("demo://external")
	app.Find("text:demo://external")
}
