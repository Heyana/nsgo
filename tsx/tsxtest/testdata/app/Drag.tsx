import { mount, signal, PanResponder, Text, View } from "nsgo";

function App() {
  const result = signal("waiting");
  const pan = PanResponder.create({
    onMoveShouldSetPanResponder: (_e, gesture) => Math.abs(gesture.dx) > 4,
    onPanResponderRelease: (_e, gesture) => result.set(`landed ${Math.round(gesture.moveX)}`),
  });
  return <View style={{ flexDirection: "row", gap: 40 }}>
    <View testID="source" {...pan.panHandlers} style={{ width: 80, height: 80 }} />
    <View testID="target" style={{ width: 80, height: 80 }} />
    <Text>{() => result()}</Text>
  </View>;
}
mount(App);
