package org.libsdl.app;

import android.content.Context;
import android.graphics.Color;
import android.graphics.Insets;
import android.os.Build;
import android.os.Bundle;
import android.view.View;
import android.view.Window;
import android.view.WindowInsets;

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

    @Override
    protected SDLSurface createSDLSurface(Context context) {
        return new Surface(context);
    }

    private static final class Surface extends SDLSurface {
        Surface(Context context) {
            super(context);
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
