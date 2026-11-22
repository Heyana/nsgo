import { Clipboard, increaseContrast, mount, openUrl, Picker, signal, Text, View } from "nsgo";
import { AlertDialog } from "nsgo/base";
import { Menu } from "nsgo/desktop";

function Services() {
  const status = signal("idle");
  return <View>
    <Text testID="status">{status}</Text>
    <Text testID="contrast">{() => increaseContrast() ? "more contrast" : "standard contrast"}</Text>
    <Text testID="fetch" onPress={async () => {
      try {
        const res = await fetch("https://api.example.com/todos?page=1", { method: "POST", body: "new" });
        const todos = await res.json();
        status.set(`${res.status} ${todos.length}`);
      } catch (e) {
        status.set(`failed: ${(e as Error).message}`);
      }
    }}>Fetch</Text>
    <Text testID="offline" onPress={() => fetch("https://elsewhere.example.com/").catch(() => status.set("offline"))}>Offline</Text>
    <Text testID="ask" onPress={async () => {
      status.set(`chose ${await AlertDialog.show({ title: "Delete?", actions: [{ label: "Cancel", role: "cancel" }, { label: "Delete", role: "destructive" }] })}`);
    }}>Ask</Text>
    <Text testID="pick" onPress={async () => {
      const picked = await Picker.pickImages({ multiple: true, limit: 3 });
      status.set(picked.map(p => `${p.file.name}:${p.file.type}:${p.file.size}`).join(","));
    }}>Pick</Text>
    <Text testID="can-take-photo" onPress={async () => {
      status.set(`camera:${await Picker.canTakePhoto()}`);
    }}>Check camera</Text>
    <Text testID="photo" onPress={async () => {
      try {
        const photo = await Picker.takePhoto({ maxDimension: 1024, quality: 0.75 });
        if (!photo) {
          status.set("photo:cancelled");
          return;
        }
        const bytes = new Uint8Array(await photo.file.arrayBuffer());
        status.set(`photo:${photo.file.name}:${photo.file.type}:${photo.file.size}:${photo.size.width}x${photo.size.height}:${Array.from(bytes).join("-")}`);
      } catch (e) {
        status.set(`photo:error:${(e as Error).message}`);
      }
    }}>Take photo</Text>
    <Text testID="photo-default" onPress={async () => {
      const photo = await Picker.takePhoto();
      status.set(photo ? `default:${photo.file.name}:${photo.file.size}` : "default:cancelled");
    }}>Take default photo</Text>
    <Text testID="open" onPress={() => openUrl("https://example.com/docs")}>Open</Text>
    <Text testID="copy" onPress={() => Clipboard.writeText("copied")}>Copy</Text>
    <Text testID="menu" onPress={() => Menu.popup([
      { label: "Rename", onSelect: () => status.set("renamed") },
      { label: "Share", submenu: [{ label: "Mail", checked: true, onSelect: () => status.set("mailed") }] },
    ])}>Menu</Text>
  </View>;
}
mount(Services);
