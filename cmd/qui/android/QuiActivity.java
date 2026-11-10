package org.libsdl.app;

import android.content.Context;
import android.graphics.Color;
import android.graphics.Insets;
import android.os.Build;
import android.os.Bundle;
import android.view.View;
import android.view.Window;
import android.view.WindowInsets;
import android.view.WindowInsetsController;

public class QuiActivity extends SDLActivity {
    @Override
    protected void onCreate(Bundle state) {
        super.onCreate(state);
        Window window = getWindow();
        if (Build.VERSION.SDK_INT >= 30) {
            window.setDecorFitsSystemWindows(false);
        }
        window.setStatusBarColor(Color.TRANSPARENT);
        window.setNavigationBarColor(Color.TRANSPARENT);
        if (Build.VERSION.SDK_INT >= 29) {
            window.setStatusBarContrastEnforced(false);
            window.setNavigationBarContrastEnforced(false);
        }
    }

    // qui sends this when the background behind the system bars turns light
    // (param 1) or dark, so the bar icons stay readable.
    private static final int COMMAND_LIGHT_BARS = 0x8001;

    @Override
    protected boolean onUnhandledMessage(int command, Object param) {
        if (command != COMMAND_LIGHT_BARS) {
            return super.onUnhandledMessage(command, param);
        }
        if (Build.VERSION.SDK_INT >= 30) {
            final int bars = WindowInsetsController.APPEARANCE_LIGHT_STATUS_BARS | WindowInsetsController.APPEARANCE_LIGHT_NAVIGATION_BARS;
            WindowInsetsController controller = getWindow().getInsetsController();
            if (controller != null) {
                controller.setSystemBarsAppearance(Integer.valueOf(1).equals(param) ? bars : 0, bars);
            }
        }
        return true;
    }

    @Override
    protected SDLSurface createSDLSurface(Context context) {
        return new Surface(context);
    }

    private static final class Surface extends SDLSurface {
        Surface(Context context) {
            super(context);
            // A key press leaves touch mode, and Android would then outline the
            // whole surface as the focused view.
            if (Build.VERSION.SDK_INT >= 26) setDefaultFocusHighlightEnabled(false);
        }

        // SDL's default also reserves the system gesture areas, which would inset
        // both sides of the app on phones using gesture navigation, and it leaves
        // out the keyboard, which an edge-to-edge window has to avoid itself.
        @Override
        public WindowInsets onApplyWindowInsets(View view, WindowInsets insets) {
            WindowInsets result = super.onApplyWindowInsets(view, insets);
            if (Build.VERSION.SDK_INT >= 30) {
                Insets area = insets.getInsets(WindowInsets.Type.systemBars() | WindowInsets.Type.displayCutout() | WindowInsets.Type.ime());
                SDLActivity.onNativeInsetsChanged(area.left, area.right, area.top, area.bottom);
            }
            return result;
        }
    }
}
