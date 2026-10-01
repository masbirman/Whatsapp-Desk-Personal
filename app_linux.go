//go:build linux

package main

/*
#cgo pkg-config: gtk+-3.0 gio-2.0
#include <gtk/gtk.h>
#include <gio/gio.h>
#include <stdlib.h>
#include <string.h>

static gboolean whatsappDeskInitialDark = FALSE;
static gboolean whatsappDeskThemeCaptured = FALSE;

static void setWhatsAppDeskGTKTheme(int mode) {
	GtkSettings* settings = gtk_settings_get_default();
	if (!settings) return;
	if (!whatsappDeskThemeCaptured) {
		g_object_get(settings, "gtk-application-prefer-dark-theme", &whatsappDeskInitialDark, NULL);
		whatsappDeskThemeCaptured = TRUE;
	}
	gboolean preferDark = mode < 0 ? whatsappDeskInitialDark : (mode == 1 ? TRUE : FALSE);
	g_object_set(settings, "gtk-application-prefer-dark-theme", preferDark, NULL);
}

// Per-monitor window placement for GTK. Monitor connectors (e.g. "DP-1")
// are stable across sessions, so each display remembers its own frame.
// The returned static buffer is valid only until the next call.
static char g_monitorNameBuf[64];

static const char* windowMonitorName(void* winPtr) {
	GtkWidget* win = GTK_WIDGET(winPtr);
	GdkWindow* gw = win ? gtk_widget_get_window(win) : NULL;
	if (!gw) return "";
	GdkDisplay* display = gdk_window_get_display(gw);
	GdkMonitor* mon = gdk_display_get_monitor_at_window(display, gw);
	if (!mon) return "";
	const char* model = gdk_monitor_get_model(mon);
	GdkRectangle geo;
	gdk_monitor_get_geometry(mon, &geo);
	snprintf(g_monitorNameBuf, sizeof(g_monitorNameBuf), "%s@%d,%d",
		model ? model : "?", geo.x, geo.y);
	return g_monitorNameBuf;
}

// Move/resize the window to a saved frame (x/y are window-manager coordinates).
static void moveWindowTo(void* winPtr, int x, int y, int w, int h) {
	GtkWidget* win = GTK_WIDGET(winPtr);
	if (!win) return;
	gtk_window_move(GTK_WINDOW(win), x, y);
	gtk_window_resize(GTK_WINDOW(win), w, h);
}

// Size-only restore for compositors (Wayland) that reject client positioning.
static void resizeWindowTo(void* winPtr, int w, int h) {
	GtkWidget* win = GTK_WIDGET(winPtr);
	if (!win) return;
	gtk_window_resize(GTK_WINDOW(win), w, h);
}

// Read the current frame. Returns 1 when the window manager reports a real
// position, 0 otherwise (Wayland never exposes absolute positions, so callers
// must treat x/y as unusable there and restore only the size).
static int getWindowFrameLinux(void* winPtr, int* x, int* y, int* w, int* h) {
	GtkWidget* win = GTK_WIDGET(winPtr);
	if (!win) return 0;
	gtk_window_get_size(GTK_WINDOW(win), w, h);
	// gtk_window_get_position returns void and never reports a position on
	// Wayland, so pre-zero x/y and report success whenever a window exists.
	*x = 0; *y = 0;
	gtk_window_get_position(GTK_WINDOW(win), x, y);
	return 1;
}

// Verify a saved frame still overlaps a connected monitor's geometry.
static int frameOnSomeMonitor(int x, int y, int w, int h) {
	GdkDisplay* display = gdk_display_get_default();
	if (!display) return 0;
	int n = gdk_display_get_n_monitors(display);
	for (int i = 0; i < n; i++) {
		GdkRectangle geo;
		gdk_monitor_get_geometry(gdk_display_get_monitor(display, i), &geo);
		int iw = (x + w < geo.x + geo.width ? x + w : geo.x + geo.width) - (x > geo.x ? x : geo.x);
		int ih = (y + h < geo.y + geo.height ? y + h : geo.y + geo.height) - (y > geo.y ? y : geo.y);
		if (iw > 60 && ih > 40) return 1;
	}
	return 0;
}

// Always-on-top must go through GTK directly: wmctrl needs X11 and a window
// manager that honours _NET_WM_STATE, so it silently does nothing on GNOME
// Wayland (Fedora default), and wmctrl is not a dependency of our packages.
static void setWhatsAppDeskKeepAbove(int enable) {
	GtkWindow* top = NULL;
	GList* toplevels = gtk_window_list_toplevels();
	for (GList* l = toplevels; l != NULL; l = l->next) {
		GtkWindow* w = GTK_WINDOW(l->data);
		if (gtk_window_get_window_type(w) == GTK_WINDOW_TOPLEVEL && !gtk_window_get_transient_for(w)) {
			top = w;
		}
	}
	g_list_free(toplevels);
	if (top) {
		gtk_window_set_keep_above(top, enable ? TRUE : FALSE);
	}
}

// --- System Tray (StatusNotifierItem via DBus) ---
//
// Minimal but spec-correct: icon + Activate-to-show + unread tooltip.
// There is deliberately NO menu: the SNI menu protocol is com.canonical
// dbusmenu (not a custom interface), and wiring its clicks back into Go
// actions cannot be verified without a Linux desktop to test on.

#define TRAY_BUS_NAME "org.kde.StatusNotifierItem-whatsapp-desk"
#define TRAY_OBJ_PATH "/StatusNotifierItem"

static GDBusConnection* g_dbus_conn = NULL;
static guint g_dbus_reg_id = 0;
static GDBusNodeInfo* g_introspection = NULL;
static char g_tray_icon_path[512] = {0};
static int g_tray_visible = 0;
static int g_unread_count = 0;
static int g_native_lock_active = 0;
static gboolean g_lock_window_minimized = FALSE;
static gint64 g_lock_last_input_us = 0;

static gboolean native_lock_track_event(GtkWidget* widget, GdkEvent* event, gpointer data) {
	(void)widget; (void)event; (void)data;
	g_lock_last_input_us = g_get_monotonic_time();
	return FALSE;
}

static gboolean native_lock_track_window_state(GtkWidget* widget, GdkEventWindowState* event, gpointer data) {
	(void)widget; (void)data;
	g_lock_window_minimized = (event->new_window_state & GDK_WINDOW_STATE_ICONIFIED) != 0;
	return FALSE;
}

static void native_lock_watch_window(void* winPtr) {
	GtkWidget* win = GTK_WIDGET(winPtr);
	if (!win) return;
	g_lock_last_input_us = g_get_monotonic_time();
	gtk_widget_add_events(win, GDK_KEY_PRESS_MASK | GDK_BUTTON_PRESS_MASK | GDK_POINTER_MOTION_MASK | GDK_SCROLL_MASK | GDK_TOUCH_MASK);
	g_signal_connect(win, "event", G_CALLBACK(native_lock_track_event), NULL);
	g_signal_connect(win, "window-state-event", G_CALLBACK(native_lock_track_window_state), NULL);
}

static gint64 native_lock_idle_milliseconds(void) {
	if (g_lock_last_input_us <= 0) return 0;
	gint64 elapsed = g_get_monotonic_time() - g_lock_last_input_us;
	return elapsed > 0 ? elapsed / 1000 : 0;
}

static int native_lock_is_minimized(void) {
	return g_lock_window_minimized ? 1 : 0;
}

static void native_lock_set_window_visible(void* winPtr, int visible) {
	GtkWidget* win = GTK_WIDGET(winPtr);
	if (!win) return;
	g_native_lock_active = visible ? 0 : 1;
	if (visible) {
		gtk_widget_show_all(win);
		gtk_window_present(GTK_WINDOW(win));
	} else {
		gtk_widget_hide(win);
	}
}

static int native_lock_prompt(void* ownerPtr, const char* title, const char* message, int password, char** result) {
	GtkWindow* owner = ownerPtr ? GTK_WINDOW(ownerPtr) : NULL;
	GtkDialogFlags flags = GTK_DIALOG_MODAL;
	if (owner) flags |= GTK_DIALOG_DESTROY_WITH_PARENT;
	GtkWidget* dialog = gtk_dialog_new_with_buttons(title, owner, flags,
		"Cancel", GTK_RESPONSE_CANCEL, "Continue", GTK_RESPONSE_OK, NULL);
	if (!dialog) return 0;
	gtk_window_set_keep_above(GTK_WINDOW(dialog), TRUE);
	gtk_window_set_position(GTK_WINDOW(dialog), GTK_WIN_POS_CENTER);
	GtkWidget* content = gtk_dialog_get_content_area(GTK_DIALOG(dialog));
	gtk_container_set_border_width(GTK_CONTAINER(content), 18);
	gtk_box_set_spacing(GTK_BOX(content), 12);
	GtkWidget* label = gtk_label_new(message);
	gtk_label_set_line_wrap(GTK_LABEL(label), TRUE);
	gtk_label_set_xalign(GTK_LABEL(label), 0.0f);
	gtk_label_set_selectable(GTK_LABEL(label), TRUE);
	gtk_box_pack_start(GTK_BOX(content), label, FALSE, FALSE, 0);
	GtkWidget* entry = gtk_entry_new();
	gtk_entry_set_visibility(GTK_ENTRY(entry), password ? FALSE : TRUE);
	gtk_entry_set_max_length(GTK_ENTRY(entry), 128);
	gtk_entry_set_activates_default(GTK_ENTRY(entry), TRUE);
	gtk_box_pack_start(GTK_BOX(content), entry, FALSE, FALSE, 0);
	gtk_dialog_set_default_response(GTK_DIALOG(dialog), GTK_RESPONSE_OK);
	gtk_widget_show_all(dialog);
	gtk_widget_grab_focus(entry);
	int response = gtk_dialog_run(GTK_DIALOG(dialog));
	if (response == GTK_RESPONSE_OK && result) {
		const char* value = gtk_entry_get_text(GTK_ENTRY(entry));
		*result = g_strdup(value ? value : "");
	}
	gtk_entry_set_text(GTK_ENTRY(entry), "");
	gtk_widget_destroy(dialog);
	return response == GTK_RESPONSE_OK ? 1 : 0;
}

static int native_lock_choice(void* ownerPtr, const char* title, const char* message, const char* yesLabel, const char* noLabel) {
	GtkWindow* owner = ownerPtr ? GTK_WINDOW(ownerPtr) : NULL;
	GtkWidget* dialog = gtk_dialog_new_with_buttons(title, owner, GTK_DIALOG_MODAL,
		"Cancel", 0, noLabel, 2, yesLabel, 1, NULL);
	if (!dialog) return 0;
	gtk_window_set_keep_above(GTK_WINDOW(dialog), TRUE);
	gtk_window_set_position(GTK_WINDOW(dialog), GTK_WIN_POS_CENTER);
	GtkWidget* content = gtk_dialog_get_content_area(GTK_DIALOG(dialog));
	gtk_container_set_border_width(GTK_CONTAINER(content), 18);
	GtkWidget* label = gtk_label_new(message);
	gtk_label_set_line_wrap(GTK_LABEL(label), TRUE);
	gtk_label_set_selectable(GTK_LABEL(label), TRUE);
	gtk_box_pack_start(GTK_BOX(content), label, TRUE, TRUE, 8);
	gtk_widget_show_all(dialog);
	int response = gtk_dialog_run(GTK_DIALOG(dialog));
	gtk_widget_destroy(dialog);
	return response == 1 ? 1 : (response == 2 ? 2 : 0);
}

static void native_lock_inform(void* ownerPtr, const char* title, const char* message) {
	GtkWindow* owner = ownerPtr ? GTK_WINDOW(ownerPtr) : NULL;
	GtkWidget* dialog = gtk_message_dialog_new(owner, GTK_DIALOG_MODAL, GTK_MESSAGE_INFO, GTK_BUTTONS_OK, "%s", message);
	gtk_window_set_title(GTK_WINDOW(dialog), title);
	gtk_window_set_keep_above(GTK_WINDOW(dialog), TRUE);
	gtk_dialog_run(GTK_DIALOG(dialog));
	gtk_widget_destroy(dialog);
}

static int native_lock_recovery_notice(void* ownerPtr, const char* title, const char* message) {
	GtkWindow* owner = ownerPtr ? GTK_WINDOW(ownerPtr) : NULL;
	GtkWidget* dialog = gtk_message_dialog_new(owner, GTK_DIALOG_MODAL, GTK_MESSAGE_INFO, GTK_BUTTONS_NONE, "%s", message);
	gtk_window_set_title(GTK_WINDOW(dialog), title);
	gtk_window_set_keep_above(GTK_WINDOW(dialog), TRUE);
	gtk_dialog_add_button(GTK_DIALOG(dialog), "Cancel", 0);
	gtk_dialog_add_button(GTK_DIALOG(dialog), "I saved the code", 1);
	int response = gtk_dialog_run(GTK_DIALOG(dialog));
	gtk_widget_destroy(dialog);
	return response == 1 ? 1 : 0;
}

static const gchar tray_introspection_xml[] =
	"<node>"
	"  <interface name='org.kde.StatusNotifierItem'>"
	"    <method name='Activate'>"
	"      <arg name='x' type='i' direction='in'/>"
	"      <arg name='y' type='i' direction='in'/>"
	"    </method>"
	"    <method name='SecondaryActivate'>"
	"      <arg name='x' type='i' direction='in'/>"
	"      <arg name='y' type='i' direction='in'/>"
	"    </method>"
	"    <method name='Scroll'>"
	"      <arg name='delta' type='i' direction='in'/>"
	"      <arg name='orientation' type='i' direction='in'/>"
	"    </method>"
	"    <method name='ContextMenu'>"
	"      <arg name='x' type='i' direction='in'/>"
	"      <arg name='y' type='i' direction='in'/>"
	"    </method>"
	"    <property name='Id' type='s' access='read'/>"
	"    <property name='Title' type='s' access='read'/>"
	"    <property name='Status' type='s' access='read'/>"
	"    <property name='IconName' type='s' access='read'/>"
	"    <property name='IconPixmap' type='a(iiay)' access='read'/>"
	"    <property name='OverlayIconName' type='s' access='read'/>"
	"    <property name='OverlayIconPixmap' type='a(iiay)' access='read'/>"
	"    <property name='AttentionIconName' type='s' access='read'/>"
	"    <property name='AttentionIconPixmap' type='a(iiay)' access='read'/>"
	"    <property name='AttentionMovieName' type='s' access='read'/>"
	"    <property name='ToolTip' type='(sa(iiay)ss)' access='read'/>"
	"    <property name='Category' type='s' access='read'/>"
	"    <signal name='NewTitle'/>"
	"    <signal name='NewStatus'/>"
	"    <signal name='NewIcon'/>"
	"    <signal name='NewAttentionIcon'/>"
	"    <signal name='NewOverlayIcon'/>"
	"    <signal name='NewToolTip'/>"
	"  </interface>"
	"  <interface name='org.freedesktop.DBus.Properties'>"
	"    <method name='Get'>"
	"      <arg name='interface' type='s' direction='in'/>"
	"      <arg name='property' type='s' direction='in'/>"
	"      <arg name='value' type='v' direction='out'/>"
	"    </method>"
	"    <method name='GetAll'>"
	"      <arg name='interface' type='s' direction='in'/>"
	"      <arg name='props' type='a{sv}' direction='out'/>"
	"    </method>"
	"    <method name='Set'>"
	"      <arg name='interface' type='s' direction='in'/>"
	"      <arg name='property' type='s' direction='in'/>"
	"      <arg name='value' type='v' direction='in'/>"
	"    </method>"
	"    <signal name='PropertiesChanged'>"
	"      <arg name='interface' type='s'/>"
	"      <arg name='changed_properties' type='a{sv}'/>"
	"      <arg name='invalidated_properties' type='as'/>"
	"    </signal>"
	"  </interface>"
	"</node>";

static char g_overlay_icon_name[64] = {0};
static int g_has_overlay = 0;

static void tray_show_window(void) {
	if (g_native_lock_active) return;
	GtkWindow* top = NULL;
	GList* toplevels = gtk_window_list_toplevels();
	for (GList* l = toplevels; l != NULL; l = l->next) {
		GtkWindow* w = GTK_WINDOW(l->data);
		if (gtk_window_get_window_type(w) == GTK_WINDOW_TOPLEVEL && !gtk_window_get_transient_for(w)) {
			top = w;
		}
	}
	g_list_free(toplevels);
	if (top) {
		gtk_window_deiconify(top);
		gtk_window_present(top);
	}
}

// Builds a valid empty IconPixmap array ("a(iiay)"). Must use a
// GVariantBuilder: g_variant_new_from_data() with a NULL buffer yields an
// invalid value, and passing a GVariant* where a tuple format expects inline
// array elements aborts the process (GLib-ERROR) as soon as a tray host
// queries the property. Returns a floating reference.
static GVariant* tray_empty_pixmap(void) {
	GVariantBuilder builder;
	g_variant_builder_init(&builder, G_VARIANT_TYPE("a(iiay)"));
	return g_variant_builder_end(&builder);
}

static GVariant* tray_get_property(GDBusConnection* conn, const gchar* sender, const gchar* object_path, const gchar* interface, const gchar* property, GError** error, gpointer user_data) {
	(void)conn; (void)sender; (void)object_path; (void)user_data;
	if (strcmp(interface, "org.kde.StatusNotifierItem") == 0) {
		if (strcmp(property, "Id") == 0) {
			return g_variant_new_string("whatsapp-desk");
		}
		if (strcmp(property, "Title") == 0) {
			return g_variant_new_string("WhatsApp Desk");
		}
		if (strcmp(property, "Status") == 0) {
			return g_variant_new_string("Active");
		}
		if (strcmp(property, "Category") == 0) {
			return g_variant_new_string("ApplicationStatus");
		}
		if (strcmp(property, "IconName") == 0) {
			return g_variant_new_string("whatsapp-desk");
		}
		if (strcmp(property, "IconPixmap") == 0) {
			return tray_empty_pixmap();
		}
		if (strcmp(property, "OverlayIconName") == 0) {
			if (g_has_overlay && g_overlay_icon_name[0]) {
				return g_variant_new_string(g_overlay_icon_name);
			}
			return g_variant_new_string("");
		}
		if (strcmp(property, "OverlayIconPixmap") == 0) {
			return tray_empty_pixmap();
		}
		if (strcmp(property, "ToolTip") == 0) {
			GVariant* empty = tray_empty_pixmap();
			char tip[96];
			if (g_unread_count > 0) snprintf(tip, sizeof(tip), "%d unread message(s)", g_unread_count);
			else snprintf(tip, sizeof(tip), "No unread messages");
			// '@' consumes the floating pixmap ref; without it the pointer
			// would be misread as inline array elements (see above).
			return g_variant_new("(s@a(iiay)ss)", "whatsapp-desk", empty, "WhatsApp Desk", tip);
		}
	}
	g_set_error(error, G_DBUS_ERROR, G_DBUS_ERROR_UNKNOWN_PROPERTY, "Unknown property %s.%s", interface, property);
	return NULL;
}

static void tray_method_call(GDBusConnection* conn, const gchar* sender, const gchar* object_path,
                             const gchar* interface, const gchar* method,
                             GVariant* params, GDBusMethodInvocation* invocation, gpointer user_data) {
	(void)conn; (void)sender; (void)object_path; (void)params; (void)user_data;
	if (strcmp(interface, "org.kde.StatusNotifierItem") == 0) {
		if (strcmp(method, "Activate") == 0) {
			tray_show_window();
			g_dbus_method_invocation_return_value(invocation, g_variant_new("()"));
			return;
		}
		if (strcmp(method, "SecondaryActivate") == 0 || strcmp(method, "ContextMenu") == 0 || strcmp(method, "Scroll") == 0) {
			g_dbus_method_invocation_return_value(invocation, g_variant_new("()"));
			return;
		}
	}
	if (strcmp(interface, "org.freedesktop.DBus.Properties") == 0) {
		if (strcmp(method, "Get") == 0) {
			const gchar* iface;
			const gchar* prop;
			g_variant_get(params, "(&s&s)", &iface, &prop);
			GError* error = NULL;
			GVariant* value = tray_get_property(conn, sender, object_path, iface, prop, &error, user_data);
			if (error) {
				// error->message must be a "%s" argument, never the format
				// string itself: a property name containing '%' (reachable
				// from any DBus client) would otherwise be interpreted as a
				// format specifier (-Wformat-security).
				g_dbus_method_invocation_return_error(invocation, G_DBUS_ERROR, G_DBUS_ERROR_UNKNOWN_PROPERTY, "%s", error->message);
				g_error_free(error);
			} else {
				g_dbus_method_invocation_return_value(invocation, g_variant_new_tuple(&value, 1));
			}
			return;
		}
		if (strcmp(method, "GetAll") == 0) {
			const gchar* iface;
			g_variant_get(params, "(&s)", &iface);
			GVariantBuilder builder;
			g_variant_builder_init(&builder, G_VARIANT_TYPE("a{sv}"));

			const gchar* props[] = {"Id", "Title", "Status", "Category", "IconName", "IconPixmap", "OverlayIconName", "OverlayIconPixmap", "ToolTip", NULL};
			for (int i = 0; props[i]; i++) {
				GError* error = NULL;
				GVariant* value = tray_get_property(conn, sender, object_path, iface, props[i], &error, user_data);
				if (!error && value) {
					g_variant_builder_add(&builder, "{sv}", props[i], value);
				}
				if (error) g_error_free(error);
			}
			GVariant* dict = g_variant_builder_end(&builder);
			g_dbus_method_invocation_return_value(invocation, g_variant_new_tuple(&dict, 1));
			return;
		}
	}
	return;
}

static const GDBusInterfaceVTable tray_vtable = {
	.method_call = tray_method_call,
	.get_property = tray_get_property,
	.set_property = NULL,
};

static void tray_emit(const char* signal) {
	if (!g_dbus_conn) return;
	GError* error = NULL;
	g_dbus_connection_emit_signal(g_dbus_conn, NULL,
		TRAY_OBJ_PATH,
		"org.kde.StatusNotifierItem",
		signal,
		NULL, &error);
	if (error) g_error_free(error);
}

static void tray_update_overlay_icon(int count) {
	g_unread_count = count > 0 ? count : 0;
	if (count > 0) {
		g_has_overlay = 1;
		snprintf(g_overlay_icon_name, sizeof(g_overlay_icon_name), "whatsapp-desk-unread-%d", count);
	} else {
		g_has_overlay = 0;
		g_overlay_icon_name[0] = 0;
	}
	tray_emit("NewOverlayIcon");
	tray_emit("NewToolTip");
	tray_emit("NewTitle");
}

// Exported for Go
void tray_update_overlay_icon_go(int count) {
	tray_update_overlay_icon(count);
}

static void tray_on_bus_acquired(GDBusConnection* conn, const gchar* name, gpointer user_data) {
	g_dbus_conn = conn;
	g_introspection = g_dbus_node_info_new_for_xml(tray_introspection_xml, NULL);

	// Register StatusNotifierItem object, then announce our bus name to
	// the watcher. RegisterStatusNotifierItem takes a single service-name
	// string (our owned bus name), NOT an (interface, path) tuple.
	GError* error = NULL;
	g_dbus_reg_id = g_dbus_connection_register_object(conn,
		TRAY_OBJ_PATH,
		g_introspection->interfaces[0],
		&tray_vtable,
		NULL, NULL, &error);
	if (error) {
		g_print("Failed to register StatusNotifierItem: %s\n", error->message);
		g_error_free(error);
		return;
	}

	GVariant* params = g_variant_new("(s)", TRAY_BUS_NAME);
	GError* callError = NULL;
	g_dbus_connection_call_sync(conn,
		"org.kde.StatusNotifierWatcher",
		"/StatusNotifierWatcher",
		"org.kde.StatusNotifierWatcher",
		"RegisterStatusNotifierItem",
		params, NULL, G_DBUS_CALL_FLAGS_NONE, -1, NULL, &callError);
	if (callError) {
		g_print("StatusNotifierWatcher register failed (no tray host?): %s\n", callError->message);
		g_error_free(callError);
	}

	g_tray_visible = 1;
}

static void tray_on_name_lost(GDBusConnection* conn, const gchar* name, gpointer user_data) {
	if (g_dbus_reg_id) {
		g_dbus_connection_unregister_object(conn, g_dbus_reg_id);
		g_dbus_reg_id = 0;
	}
	if (g_introspection) {
		g_dbus_node_info_unref(g_introspection);
		g_introspection = NULL;
	}
	g_tray_visible = 0;
}

static void tray_init(const char* icon_path) {
	if (icon_path && icon_path[0]) {
		strncpy(g_tray_icon_path, icon_path, sizeof(g_tray_icon_path) - 1);
	}

	g_bus_own_name(G_BUS_TYPE_SESSION,
		TRAY_BUS_NAME,
		G_BUS_NAME_OWNER_FLAGS_NONE,
		tray_on_bus_acquired,
		NULL,
		tray_on_name_lost,
		NULL, NULL);
}

static void tray_update_icon(const char* icon_path) {
	if (icon_path && icon_path[0] && g_dbus_conn && g_tray_visible) {
		strncpy(g_tray_icon_path, icon_path, sizeof(g_tray_icon_path) - 1);
		tray_emit("NewIcon");
	}
}

static void tray_shutdown(void) {
	if (g_dbus_conn && g_tray_visible) {
		// Unregister from watcher (single service-name string)
		GVariant* params = g_variant_new("(s)", TRAY_BUS_NAME);
		g_dbus_connection_call_sync(g_dbus_conn,
			"org.kde.StatusNotifierWatcher",
			"/StatusNotifierWatcher",
			"org.kde.StatusNotifierWatcher",
			"UnregisterStatusNotifierItem",
			params, NULL, G_DBUS_CALL_FLAGS_NONE, -1, NULL, NULL);

		if (g_dbus_reg_id) {
			g_dbus_connection_unregister_object(g_dbus_conn, g_dbus_reg_id);
			g_dbus_reg_id = 0;
		}
		if (g_introspection) {
			g_dbus_node_info_unref(g_introspection);
			g_introspection = NULL;
		}
		g_tray_visible = 0;
	}
}
*/
import "C"

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
	"syscall"
	"time"
	"unsafe"

	webview "github.com/webview/webview_go"
)

//go:embed icon.png
var embeddedIconPNG []byte

const (
	userAgentLinux = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36"
)

var (
	isAlwaysOnTopLinux = false

	linuxCacheHomes   []string
	linuxPurgeTargets []string
)

func applyNativeThemeLinux(theme string) {
	mode := C.int(-1)
	if theme == "dark" {
		mode = 1
	} else if theme == "light" {
		mode = 0
	}
	C.setWhatsAppDeskGTKTheme(mode)
}

func nativeLockCString(value string) *C.char {
	return C.CString(value)
}

func nativeLockFreeCString(value *C.char, clear bool) {
	if value == nil {
		return
	}
	if clear {
		C.memset(unsafe.Pointer(value), 0, C.strlen(value))
	}
	C.free(unsafe.Pointer(value))
}

func nativeCredentialPrompt(owner uintptr, title, message string, password bool) ([]byte, bool) {
	cTitle, cMessage := nativeLockCString(title), nativeLockCString(message)
	defer nativeLockFreeCString(cTitle, false)
	defer nativeLockFreeCString(cMessage, false)
	var output *C.char
	accepted := C.native_lock_prompt(unsafe.Pointer(owner), cTitle, cMessage, C.int(b2i(password)), &output) != 0
	if output == nil {
		return nil, false
	}
	defer nativeLockFreeCString(output, true)
	length := C.strlen(output)
	value := C.GoBytes(unsafe.Pointer(output), C.int(length))
	return value, accepted
}

func nativeTextPrompt(owner uintptr, title, message string) ([]byte, bool) {
	return nativeCredentialPrompt(owner, title, message, false)
}

func nativeAskChoice(owner uintptr, title, message, yesLabel, noLabel string) int {
	cTitle, cMessage := nativeLockCString(title), nativeLockCString(message)
	cYes, cNo := nativeLockCString(yesLabel), nativeLockCString(noLabel)
	defer nativeLockFreeCString(cTitle, false)
	defer nativeLockFreeCString(cMessage, false)
	defer nativeLockFreeCString(cYes, false)
	defer nativeLockFreeCString(cNo, false)
	return int(C.native_lock_choice(unsafe.Pointer(owner), cTitle, cMessage, cYes, cNo))
}

func nativeInform(owner uintptr, title, message string) {
	cTitle, cMessage := nativeLockCString(title), nativeLockCString(message)
	defer nativeLockFreeCString(cTitle, false)
	defer nativeLockFreeCString(cMessage, false)
	C.native_lock_inform(unsafe.Pointer(owner), cTitle, cMessage)
}

func nativeShowRecoveryCode(owner uintptr, code string) bool {
	message := "Save this one-time recovery code somewhere private. It is shown only once. Use the button only after saving it.\n\n" + code
	cTitle, cMessage := nativeLockCString("Save recovery code"), nativeLockCString(message)
	defer nativeLockFreeCString(cTitle, false)
	defer nativeLockFreeCString(cMessage, true)
	return C.native_lock_recovery_notice(unsafe.Pointer(owner), cTitle, cMessage) != 0
}

func nativeSetMainWindowVisible(owner uintptr, visible bool) {
	C.native_lock_set_window_visible(unsafe.Pointer(owner), C.int(b2i(visible)))
}

func nativeWindowMinimized(owner uintptr) bool {
	_ = owner
	return C.native_lock_is_minimized() != 0
}

func nativeIdleFor(owner uintptr) time.Duration {
	_ = owner
	return time.Duration(C.native_lock_idle_milliseconds()) * time.Millisecond
}

func startLinuxLockActivityTracking(owner uintptr) {
	C.native_lock_watch_window(unsafe.Pointer(owner))
}

func checkSingleInstance() (*os.File, bool) {
	dataDir := getUserDataDir()
	lockPath := filepath.Join(dataDir, "app.lock")
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, true
	}
	err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		// Already running
		return nil, false
	}
	return file, true
}

func getUserDataDir() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		home, _ := os.UserHomeDir()
		configDir = filepath.Join(home, ".config")
	}
	dir := filepath.Join(configDir, "WhatsAppDesk")
	_ = os.MkdirAll(dir, 0755)
	return dir
}

func ensureAppIconFileLinux(dir string) string {
	iconPath := filepath.Join(dir, "app_icon.png")
	if _, err := os.Stat(iconPath); os.IsNotExist(err) && len(embeddedIconPNG) > 0 {
		_ = os.WriteFile(iconPath, embeddedIconPNG, 0644)
	}
	return iconPath
}

func showNativeNotification(title, message, iconPath string) {
	if iconPath != "" {
		_ = exec.Command("notify-send", "-a", "WhatsApp Desk", "-i", iconPath, title, message).Run()
	} else {
		_ = exec.Command("notify-send", "-a", "WhatsApp Desk", title, message).Run()
	}
}

func toggleAlwaysOnTopLinux() bool {
	isAlwaysOnTopLinux = !isAlwaysOnTopLinux
	// Native GTK path works on both X11 and Wayland; wmctrl is only a last
	// resort for exotic setups and must never be the primary mechanism.
	C.setWhatsAppDeskKeepAbove(C.int(b2i(isAlwaysOnTopLinux)))
	if !isAlwaysOnTopLinux {
		if path, err := exec.LookPath("wmctrl"); err == nil && path != "" {
			_ = exec.Command("wmctrl", "-r", windowTitle, "-b", "remove,above").Run()
		}
	}
	return isAlwaysOnTopLinux
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// System Tray bindings
func initSystemTrayLinux(iconPath string) {
	cPath := C.CString(iconPath)
	defer C.free(unsafe.Pointer(cPath))
	C.tray_init(cPath)
}

func shutdownSystemTrayLinux() {
	C.tray_shutdown()
}

func getAutoStartDesktopPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "autostart", "whatsapp-desk.desktop")
}

func toggleAutoStartLinux() bool {
	p := getAutoStartDesktopPath()
	if p == "" {
		return false
	}
	if _, err := os.Stat(p); err == nil {
		_ = os.Remove(p)
		return false
	}

	execPath, err := os.Executable()
	if err != nil {
		return false
	}

	_ = os.MkdirAll(filepath.Dir(p), 0755)
	desktopContent := fmt.Sprintf(`[Desktop Entry]
Type=Application
Version=1.0
Name=WhatsApp Desk
Comment=Lightweight WhatsApp Desktop Client
Exec=%s
Icon=whatsapp-desk
Terminal=false
Categories=Network;InstantMessaging;
StartupNotify=true
`, execPath)

	err = os.WriteFile(p, []byte(desktopContent), 0644)
	return err == nil
}

// readWindowStateFile loads and validates the persisted window state.
func readWindowStateFile(dir string) *WindowState {
	data, err := os.ReadFile(filepath.Join(dir, "window_state.json"))
	if err != nil {
		return nil
	}
	var state WindowState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil
	}
	return &state
}

// loadWindowStateForMonitor picks the frame to restore for the monitor the
// window currently sits on. Wayland compositors never expose absolute window
// positions (gtk_window_get_position returns FALSE there), so state saved on
// Wayland carries size only — X and Y are both zero. Such entries are always
// accepted and restored as size-only, letting the compositor place the window
// itself. X11 frames are verified to still overlap a connected monitor so a
// disconnected display can never strand the window off-screen.
func loadWindowStateForMonitor(dir, monitorKey string) *WindowState {
	state := readWindowStateFile(dir)
	if state == nil {
		return nil
	}
	validSize := func(s WindowState) bool { return s.Width >= 450 && s.Height >= 320 }

	if monitorKey != "" {
		if per, ok := state.Screens[monitorKey]; ok && validSize(per) {
			if per.X == 0 && per.Y == 0 {
				return &per
			}
			if C.frameOnSomeMonitor(C.int(per.X), C.int(per.Y), C.int(per.Width), C.int(per.Height)) != 0 {
				return &per
			}
			// Saved frame no longer lands on any monitor: keep the size, but
			// let the window manager choose a position.
			return &WindowState{Width: per.Width, Height: per.Height}
		}
	}
	if validSize(*state) {
		if state.X == 0 && state.Y == 0 {
			return state
		}
		if C.frameOnSomeMonitor(C.int(state.X), C.int(state.Y), C.int(state.Width), C.int(state.Height)) != 0 {
			return state
		}
		return &WindowState{Width: state.Width, Height: state.Height}
	}
	return nil
}

// saveWindowState records the live frame. On X11 the frame is stored per
// monitor (keyed by connector|model so it survives replugging) alongside the
// legacy top-level fields; on Wayland only the size is meaningful and only the
// legacy fields are written. fallbackWidth/Height come from the page's resize
// event and cover the window not being realized yet.
func saveWindowState(dir string, win unsafe.Pointer, fallbackWidth, fallbackHeight int) {
	if win == nil {
		return
	}
	var x, y, w, h C.int
	hasPosition := C.getWindowFrameLinux(win, &x, &y, &w, &h) != 0
	if w < 450 || h < 320 {
		// Not realized yet (or a transient zero frame): trust the page size.
		w = C.int(fallbackWidth)
		h = C.int(fallbackHeight)
		hasPosition = false
	}
	if w < 450 || h < 320 {
		return
	}

	state := readWindowStateFile(dir)
	if state == nil {
		state = &WindowState{}
	}
	if state.Screens == nil {
		state.Screens = map[string]WindowState{}
	}

	if !hasPosition {
		if state.Width == float64(w) && state.Height == float64(h) {
			return // size unchanged — skip the disk write
		}
		state.Width = float64(w)
		state.Height = float64(h)
	} else {
		monitorKey := C.GoString(C.windowMonitorName(win))
		entry := WindowState{X: float64(x), Y: float64(y), Width: float64(w), Height: float64(h)}
		if monitorKey != "" {
			if prev, ok := state.Screens[monitorKey]; ok &&
				prev.X == entry.X && prev.Y == entry.Y &&
				prev.Width == entry.Width && prev.Height == entry.Height {
				return // unchanged on this monitor — skip the disk write
			}
			state.Screens[monitorKey] = entry
		}
		state.X = entry.X
		state.Y = entry.Y
		state.Width = entry.Width
		state.Height = entry.Height
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err == nil {
		_ = os.WriteFile(filepath.Join(dir, "window_state.json"), data, 0644)
	}
}

func runApp() {
	lockFile, isSingle := checkSingleInstance()
	if !isSingle {
		fmt.Println("WhatsApp Desk is already running.")
		os.Exit(0)
	}
	if lockFile != nil {
		defer lockFile.Close()
	}

	userDataDir := getUserDataDir()
	cacheDebugLog("startup: pid=%d profile=%s", os.Getpid(), userDataDir)
	// WebKitGTK writes its network disk cache under ~/.cache keyed by app name.
	// Also covers WebKitGTK 4.0/4.1 (webkitgtk-4.0) and 6.0 (webkitgtk-6.0) cache directories.
	home, _ := os.UserHomeDir()
	linuxCacheHomes = []string{
		filepath.Join(home, ".cache", "whatsapp-desk"),
		filepath.Join(home, ".cache", "WhatsAppDesk"),
		filepath.Join(home, ".cache", "webkitgtk-4.0"),
		filepath.Join(home, ".cache", "webkitgtk-6.0"),
	}
	// Purge targets include NetworkCache subdirectories where HTTP cache lives
	linuxPurgeTargets = []string{
		filepath.Join(home, ".cache", "whatsapp-desk"),
		filepath.Join(home, ".cache", "WhatsAppDesk"),
		filepath.Join(home, ".cache", "webkitgtk-4.0", "NetworkCache"),
		filepath.Join(home, ".cache", "webkitgtk-6.0", "NetworkCache"),
	}
	enforceDiskCacheCapFrom(linuxCacheHomes, linuxPurgeTargets, "startup")

	// Restore window state if previously saved. The monitor the window will
	// land on is only known once the window is realized, so this first pass
	// applies the legacy top-level frame (position on X11, size everywhere)
	// and a second pass below refines it to the per-monitor entry.
	initialWidth := windowWidth
	initialHeight := windowHeight
	if state := readWindowStateFile(userDataDir); state != nil {
		if state.Width >= 450 && state.Height >= 320 {
			initialWidth = int(state.Width)
			initialHeight = int(state.Height)
		}
	}

	w := webview.New(false)
	if w == nil {
		log.Fatalln("Gagal inisialisasi WebKitGTK Webview")
	}
	defer w.Destroy()
	startLinuxLockActivityTracking(uintptr(w.Window()))
	applyNativeThemeLinux(loadSettings().Theme)

	w.SetTitle(windowTitle)
	w.SetSize(initialWidth, initialHeight, webview.HintNone)

	// Bind window state saver from JS resize events. The window handle is
	// passed through so X11 frames can be stored per monitor.
	_ = w.Bind("saveWindowStateNative", func(width, height int) {
		request, err := newWindowSizeRequest(width, height)
		if err != nil {
			return
		}
		saveWindowState(userDataDir, w.Window(), request.Width, request.Height)
	})

	iconPath := ensureAppIconFileLinux(userDataDir)

	// Initialize system tray
	initSystemTrayLinux(iconPath)
	defer shutdownSystemTrayLinux()

	// Bind native notification bridge
	_ = w.Bind("sendNativeNotification", func(title, body string) {
		request, err := newNativeNotificationRequest(title, body)
		if err != nil {
			return
		}
		go showNativeNotification(request.Title, request.Body, iconPath)
	})
	_ = w.Bind("getNotificationsEnabledNative", getNotificationsEnabled)
	_ = w.Bind("setNotificationsEnabledNative", setNotificationsEnabled)

	_ = w.Bind("releaseMemoryNative", func() {
		debug.FreeOSMemory()
		debugLogProcessStats("release-memory")
		enforceDiskCacheCapFrom(linuxCacheHomes, linuxPurgeTargets, "minimize")
	})

	// Bind external link handler (xdg-open)
	_ = w.Bind("openExternalLink", func(rawURL string) {
		request, err := newExternalLinkRequest(rawURL)
		if err != nil {
			return
		}
		go func() { _ = exec.Command("xdg-open", request.URL).Start() }()
	})

	// Bind dock badge -> StatusNotifierItem overlay icon (unread count)
	_ = w.Bind("updateDockBadge", func(badge string) {
		request, err := newUnreadBadgeRequest(badge)
		if err != nil {
			return
		}
		badge = request.Value
		count := 0
		if strings.TrimSpace(badge) != "" {
			fmt.Sscanf(strings.TrimSpace(badge), "%d", &count)
			if count == 0 {
				count = 1 // non-numeric badge (e.g. "•") still means unread
			}
		}
		C.tray_update_overlay_icon_go(C.int(count))
	})

	// Bind Always on Top toggle
	_ = w.Bind("toggleAlwaysOnTopNative", func() bool {
		return toggleAlwaysOnTopLinux()
	})

	// Bind Auto-Start toggle
	_ = w.Bind("toggleAutoStartNative", func() bool {
		return toggleAutoStartLinux()
	})

	// Bind in-app auto updater
	_ = w.Bind("checkForUpdateNative", func(manual bool) UpdateInfo {
		info, err := checkForUpdate(appVersion)
		if err != nil {
			return UpdateInfo{CurrentVersion: appVersion, CheckError: err.Error()}
		}
		return *info
	})

	_ = w.Bind("startUpdateNative", func(downloadURL string) {
		go func() {
			_ = executeUpdate(w, downloadURL)
		}()
	})

	// Bind download, preview, and settings handlers
	_ = w.Bind("saveDownloadedFileNative", func(filename, dataURI string) string {
		path, err := saveDownloadedFileFromBridge(filename, dataURI)
		if err != nil {
			return ""
		}
		return path
	})

	_ = w.Bind("previewDocumentNative", func(filename, dataURI string) string {
		path, err := previewDocumentFromBridge(filename, dataURI)
		if err != nil {
			return ""
		}
		return path
	})

	_ = w.Bind("openFileNative", func(filePath string) bool {
		return openFileFromBridge(filePath)
	})

	// Lazy-load SheetJS library for spreadsheet preview
	_ = w.Bind("loadXLSXLibraryNative", func() string {
		return xlsxLibJS
	})

	_ = w.Bind("getDownloadDirNative", func() string {
		s := loadSettings()
		return s.DownloadDir
	})

	_ = w.Bind("getOrganizeByMonthNative", func() bool {
		return loadSettings().OrganizeByMonth
	})

	_ = w.Bind("setOrganizeByMonthNative", func(on bool) bool {
		return setOrganizeByMonth(on)
	})

	_ = w.Bind("checkFileExistsNative", func(filename string) bool {
		return fileExistsInDownloadDir(filename)
	})

	_ = w.Bind("chooseDownloadDirNative", func() string {
		selected, err := chooseFolderDialog()
		if err != nil || selected == "" {
			return ""
		}
		// Fast-fail here (the shared saver and loadSettings re-validate
		// anyway) so the UI never reports a sensitive folder as applied.
		if err := validateDownloadDir(selected); err != nil {
			return ""
		}
		s := loadSettings()
		s.DownloadDir = selected
		_ = saveSettings(s)
		return selected
	})

	_ = w.Bind("openDownloadDirNative", func() bool {
		s := loadSettings()
		_ = openFolderInFileManager(s.DownloadDir)
		return true
	})

	_ = w.Bind("resetDownloadDirNative", func() string {
		s := loadSettings()
		s.DownloadDir = getDefaultDownloadDir()
		_ = saveSettings(s)
		return s.DownloadDir
	})

	_ = w.Bind("getAppThemeNative", func() string {
		s := loadSettings()
		return s.Theme
	})

	_ = w.Bind("setAppThemeNative", func(theme string) string {
		request, err := newThemeChangeRequest(theme)
		if err != nil {
			return loadSettings().Theme
		}
		saved := saveTheme(request.Theme)
		applyNativeThemeLinux(saved)
		return saved
	})

	// Spell check bindings
	_ = w.Bind("getSpellCheckEnabledNative", func() bool {
		return getSpellCheckEnabled()
	})
	_ = w.Bind("setSpellCheckEnabledNative", func(enabled bool) bool {
		return setSpellCheckEnabled(enabled)
	})
	_ = w.Bind("getSpellCheckLangNative", func() string {
		return getSpellCheckLang()
	})
	_ = w.Bind("setSpellCheckLangNative", func(lang string) string {
		request, err := newSpellCheckLanguageRequest(lang)
		if err != nil {
			return getSpellCheckLang()
		}
		return setSpellCheckLang(request.Language)
	})
	_ = w.Bind("getBlurAvatarsNative", func() bool {
		return getBlurAvatars()
	})
	_ = w.Bind("setBlurAvatarsNative", func(on bool) bool {
		return setBlurAvatars(on)
	})
	_ = w.Bind("getPrivacyProfileStateNative", privacyProfileStateJSON)
	_ = w.Bind("selectPrivacyProfileNative", selectPrivacyProfileJSON)
	_ = w.Bind("copyPrivacyProfileToCustomNative", copyPrivacyProfileToCustomJSON)
	_ = w.Bind("resetPrivacyProfilesNative", resetPrivacyProfilesJSON)
	_ = w.Bind("setCustomPrivacyPolicyNative", updateCustomPrivacyPolicyJSON)
	_ = w.Bind("getLockPolicyNative", lockPolicyJSON)
	_ = w.Bind("requestAppLockNative", func() bool {
		return requestNativeAppLock(uintptr(w.Window()))
	})
	_ = w.Bind("manageAppLockNative", func() bool {
		return manageNativeAppLock(uintptr(w.Window()))
	})
	_ = w.Bind("getPendingCrashNative", func() string {
		return pendingCrashReport()
	})
	_ = w.Bind("markCrashNotifiedNative", func() bool {
		return markCrashNotified()
	})

	w.Init(getInitScript(userAgentLinux))
	if !showStartupNativeAppLock(uintptr(w.Window())) {
		return
	}
	w.Navigate(appURL)
	startNativeAppLockWatcher(w, uintptr(w.Window()))

	// Check for updates in the background after startup & periodically
	go guardGoroutine("update-ticker", func() {
		checkAndNotifyUpdate := func() {
			info, err := checkForUpdate(appVersion)
			if err == nil && info != nil && info.Available {
				w.Dispatch(func() {
					script := fmt.Sprintf("if (window.showUpdateBanner) { window.showUpdateBanner(%q, %q, %q); }",
						info.LatestVersion, info.ReleaseTitle, info.DownloadURL)
					w.Eval(script)
				})
			}
		}

		time.Sleep(5 * time.Second)
		checkAndNotifyUpdate()

		ticker := time.NewTicker(4 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			checkAndNotifyUpdate()
		}
	})

	// Second pass: once the window is realized the compositor has placed it on
	// a monitor, so the frame saved for that specific display can be applied.
	// Runs on the UI thread via Dispatch; a short delay avoids fighting the
	// window manager's own initial placement.
	go guardGoroutine("restore-monitor-frame", func() {
		time.Sleep(900 * time.Millisecond)
		w.Dispatch(func() {
			win := w.Window()
			if win == nil {
				return
			}
			monitorKey := C.GoString(C.windowMonitorName(win))
			state := loadWindowStateForMonitor(userDataDir, monitorKey)
			if state == nil {
				return
			}
			// Wayland exposes no absolute position: keep whatever placement
			// the compositor chose and only honor a saved size. gtk_window_move
			// is a no-op there, so the resize-only helper is used to avoid
			// passing coordinates the compositor never reported.
			var x, y, curW, curH C.int
			hasPosition := C.getWindowFrameLinux(win, &x, &y, &curW, &curH) != 0
			if !hasPosition {
				if int(curW) == int(state.Width) && int(curH) == int(state.Height) {
					return
				}
				C.resizeWindowTo(win, C.int(state.Width), C.int(state.Height))
				return
			}
			if int(x) == int(state.X) && int(y) == int(state.Y) &&
				int(curW) == int(state.Width) && int(curH) == int(state.Height) {
				return // already where it belongs
			}
			C.moveWindowTo(win, C.int(state.X), C.int(state.Y), C.int(state.Width), C.int(state.Height))
		})
	})

	defer saveWindowState(userDataDir, w.Window(), initialWidth, initialHeight)
	w.Run()
}
