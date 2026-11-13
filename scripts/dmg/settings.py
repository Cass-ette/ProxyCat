"""Native installer plus a manual drag-to-Applications option."""
import os

app = defines["app"]
format = "UDZO"
filesystem = "HFS+"
compression_level = 9
files = [app, defines["installer"]]
symlinks = {"Applications": "/Applications"}
icon = os.path.join(app, "Contents", "Resources", "AppIcon.icns")
background = defines["background"]
window_rect = ((200, 140), (640, 480))
default_view = "icon-view"
show_status_bar = False
show_tab_view = False
show_toolbar = False
show_pathbar = False
show_sidebar = False
sidebar_width = 0
arrange_by = None
icon_size = 88
text_size = 13
icon_locations = {"ProxyCat.app": (180, 175), "Applications": (460, 175), "安装 ProxyCat.app": (118, 355)}
