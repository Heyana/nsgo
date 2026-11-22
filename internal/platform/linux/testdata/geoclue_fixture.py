#!/usr/bin/env python3
"""Minimal private GeoClue2-compatible service for NSGo native tests."""
import dbus
import dbus.service
import dbus.mainloop.glib
from gi.repository import GLib

BUS = "org.freedesktop.GeoClue2"
MANAGER = "/org/freedesktop/GeoClue2/Manager"
CLIENT = "/org/freedesktop/GeoClue2/Client/1"
LOCATION = "/org/freedesktop/GeoClue2/Location/1"

class Manager(dbus.service.Object):
    def __init__(self, bus):
        super().__init__(bus, MANAGER)

    @dbus.service.method(BUS + ".Manager", in_signature="", out_signature="o")
    def GetClient(self):
        return CLIENT

class Client(dbus.service.Object):
    def __init__(self, bus):
        super().__init__(bus, CLIENT)
        self.location = dbus.ObjectPath("/")

    @dbus.service.method(BUS + ".Client", in_signature="", out_signature="")
    def Start(self):
        self.location = dbus.ObjectPath(LOCATION)

    @dbus.service.method(BUS + ".Client", in_signature="", out_signature="")
    def Stop(self):
        self.location = dbus.ObjectPath("/")

    @dbus.service.method("org.freedesktop.DBus.Properties", in_signature="ss", out_signature="v")
    def Get(self, interface, name):
        if name == "Location":
            return self.location
        return ""

class Location(dbus.service.Object):
    def __init__(self, bus):
        super().__init__(bus, LOCATION)

    @dbus.service.method("org.freedesktop.DBus.Properties", in_signature="ss", out_signature="v")
    def Get(self, interface, name):
        values = {"Latitude": 35.6812, "Longitude": 139.7671, "Accuracy": 8.0, "Timestamp": dbus.Int64(0)}
        return values.get(name, 0.0)

dbus.mainloop.glib.DBusGMainLoop(set_as_default=True)
bus = dbus.SessionBus()
bus.request_name(BUS)
Manager(bus)
Client(bus)
Location(bus)
GLib.MainLoop().run()
