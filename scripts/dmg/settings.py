"""Finder layout for a drag-to-Applications disk image (no GUI automation)."""
import os

app = defines["app"]
format = "UDZO"
filesystem = "HFS+"
compression_level = 9
files = [app]
symlinks = {"Applications": "/Applications"}
icon = os.path.join(app, "Contents", "Resources", "AppIcon.icns")
background = defines["background"]
window_rect = ((200, 140), (660, 440))
default_view = "icon-view"
show_status_bar = False
show_tab_view = False
show_toolbar = False
show_pathbar = False
show_sidebar = False
sidebar_width = 0
arrange_by = None
icon_size = 104
text_size = 14
icon_locations = {"ProxyCat.app": (170, 190), "Applications": (490, 190)}
