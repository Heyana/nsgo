import { mount, View, Text } from "nsgo";
import {
  checkForUpdate,
  checkInBackground,
  checkDaily,
  capability,
  status,
  setAutomaticChecks,
  setAutomaticDownloads,
} from "nsgo/updater";

function App() {
  return (
    <View>
      <Text testID="cap">{() => `${capability().managed}:${capability().store ?? "none"}`}</Text>
      <Text testID="status">
        {() => {
          const s = status();
          return `${s.automaticChecks}:${s.automaticDownloads}:${s.available ?? "none"}:${s.notes ?? "none"}:${s.phase}`;
        }}
      </Text>
      <View testID="check" style={{ width: 80, height: 30 }} onPress={() => checkForUpdate()}>
        <Text>Check</Text>
      </View>
      <View testID="bg" style={{ width: 80, height: 30 }} onPress={() => checkInBackground()}>
        <Text>Background</Text>
      </View>
      <View testID="daily" style={{ width: 80, height: 30 }} onPress={() => checkDaily()}>
        <Text>Daily</Text>
      </View>
      <View testID="auto-off" style={{ width: 80, height: 30 }} onPress={() => setAutomaticChecks(false)}>
        <Text>AutoOff</Text>
      </View>
      <View testID="auto-on" style={{ width: 80, height: 30 }} onPress={() => setAutomaticChecks(true)}>
        <Text>AutoOn</Text>
      </View>
      <View testID="dl-on" style={{ width: 80, height: 30 }} onPress={() => setAutomaticDownloads(true)}>
        <Text>DLOn</Text>
      </View>
    </View>
  );
}

mount(App);
