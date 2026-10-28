import { type JSX, mount, signal, Text, View } from "nsgo";

function Row(props: { name: string; onRemove: () => void }) {
  return (
    <View testID={"row-" + props.name} onPress={props.onRemove} style={{ flexDirection: "row" }}>
      <Text>{props.name}</Text>
    </View>
  );
}

function Card(props: { title: string; children?: JSX.Element }) {
  return (
    <View style={[{ padding: 8 }, { gap: 4 }]}>
      <Text style={{ fontWeight: "bold" }}>{props.title}</Text>
      {props.children}
    </View>
  );
}

function App() {
  const on = signal(false);
  const items = signal(["a", "b", "c"]);
  const count = signal(0);
  return (
    <View style={{ flex: 1 }}>
      <View testID="toggle" onPress={() => on.update((v) => !v)}>
        <Text>toggle</Text>
      </View>
      <If when={on()} else={<Text>off</Text>}>
        <Text>on</Text>
      </If>
      <View testID="reverse" onPress={() => items.update((l) => [...l].reverse())}>
        <Text>reverse</Text>
      </View>
      <View testID="add" onPress={() => items.update((l) => [...l, "d" + l.length])}>
        <Text>add</Text>
      </View>
      <Each of={items()}>
        {(name) => <Row name={name()} onRemove={() => items.update((l) => l.filter((x) => x !== name()))} />}
      </Each>
      <Card title={"Count " + count()}>
        <Text testID="tap" onPress={() => count.update((c) => c + 1)}>
          tap <Text style={{ color: "red" }}>{count()}</Text> times
        </Text>
        {count() > 1 ? <Text>many</Text> : null}
      </Card>
    </View>
  );
}

mount(App);
