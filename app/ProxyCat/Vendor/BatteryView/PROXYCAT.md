# BatteryView in ProxyCat

Source: https://github.com/theMomax/BatteryView
Commit: `a800142347e2fc5e344a547fb29b6d46c3dddcf1`
License: MIT, see LICENSE and the app’s ThirdPartyNotices.txt.

This is a local, unmodified copy of the upstream Swift package. Xcode builds it
offline; no remote package resolution is required. ProxyCat uses Battery with
SFSymbolStyle and supplies subscription allowance as its level. It does not use
SystemBattery or read the Mac’s battery. ColorConfiguration is implemented in
TrafficCapsuleView.swift; upstream rendering code is unchanged.
