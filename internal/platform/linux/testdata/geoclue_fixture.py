#!/usr/bin/env python3
"""A GeoClue 2 stand-in on the session bus it is started on, for NSGo's native location tests.

Each GetClient makes a new client. A started client publishes a first fix at once and a new Location object every 200 ms after it, moving north by 0.001 degrees each time. Start fails with AccessDenied while the file named by GEOCLUE_FIXTURE_DENY exists. Each call is appended to the file named by GEOCLUE_FIXTURE_LOG as one line.
"""
import os
import time

import dbus
import dbus.mainloop.glib
import dbus.service
from gi.repository import GLib

BUS = "org.freedesktop.GeoClue2"
ROOT = "/org/freedesktop/GeoClue2"
PROPERTIES = "org.freedesktop.DBus.Properties"


def log(line):
    with open(os.environ["GEOCLUE_FIXTURE_LOG"], "a") as f:
        f.write(line + "\n")


class Location(dbus.service.Object):
    def __init__(self, bus, path, n):
        super().__init__(bus, path)
        self.values = {
            "Latitude": dbus.Double(35.6812 + 0.001 * (n - 1)),
            "Longitude": dbus.Double(139.7671),
            "Accuracy": dbus.Double(8.0),
            "Altitude": dbus.Double(-1.7976931348623157e308),
            "Speed": dbus.Double(-1.0),
            "Heading": dbus.Double(90.0),
            "Timestamp": dbus.Struct((dbus.UInt64(int(time.time())), dbus.UInt64(0)), signature="tt"),
        }

    @dbus.service.method(PROPERTIES, in_signature="ss", out_signature="v")
    def Get(self, interface, name):
        return self.values[name]


class Client(dbus.service.Object):
    def __init__(self, bus, manager, path):
        super().__init__(bus, path)
        self.bus, self.manager, self.path = bus, manager, path
        self.location = dbus.ObjectPath("/")
        self.props = {}
        self.timer = 0

    def publish(self):
        self.manager.fixes += 1
        path = "%s/Location/%d" % (ROOT, self.manager.fixes)
        Location(self.bus, path, self.manager.fixes)
        self.location = dbus.ObjectPath(path)
        return True

    @dbus.service.method(BUS + ".Client", in_signature="", out_signature="")
    def Start(self):
        deny = os.environ.get("GEOCLUE_FIXTURE_DENY", "")
        if deny and os.path.exists(deny):
            log("denied " + self.path)
            raise dbus.exceptions.DBusException("'%s' disallowed by the fixture" % self.props.get("DesktopId", ""), name="org.freedesktop.DBus.Error.AccessDenied")
        log("start %s %s %s" % (self.path, self.props.get("DesktopId", ""), self.props.get("RequestedAccuracyLevel", "")))
        self.publish()
        self.timer = GLib.timeout_add(200, self.publish)

    @dbus.service.method(BUS + ".Client", in_signature="", out_signature="")
    def Stop(self):
        log("stop " + self.path)
        if self.timer:
            GLib.source_remove(self.timer)
            self.timer = 0
        self.location = dbus.ObjectPath("/")

    @dbus.service.method(PROPERTIES, in_signature="ss", out_signature="v")
    def Get(self, interface, name):
        if name == "Location":
            return self.location
        return self.props[name]

    @dbus.service.method(PROPERTIES, in_signature="ssv", out_signature="")
    def Set(self, interface, name, value):
        self.props[name] = value


class Manager(dbus.service.Object):
    def __init__(self, bus):
        super().__init__(bus, ROOT + "/Manager")
        self.bus = bus
        self.clients = 0
        self.fixes = 0

    @dbus.service.method(BUS + ".Manager", in_signature="", out_signature="o")
    def GetClient(self):
        self.clients += 1
        path = "%s/Client/%d" % (ROOT, self.clients)
        Client(self.bus, self, path)
        return dbus.ObjectPath(path)


dbus.mainloop.glib.DBusGMainLoop(set_as_default=True)
bus = dbus.SessionBus()
manager = Manager(bus)
bus.request_name(BUS)
print("ready", flush=True)
GLib.MainLoop().run()
