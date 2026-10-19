import QtQuick 2.6
import Sailfish.Silica 1.0
import eu.ftuna 1.0
import "pages"

ApplicationWindow {
    Ftuna {
        id: ftuna
    }

    allowedOrientations: Orientation.All
    initialPage: ftuna.onboarded ? mainComponent : welcomeComponent

    Component {
        id: welcomeComponent
        WelcomePage {
            backend: ftuna
        }
    }
    Component {
        id: mainComponent
        MainPage {
            backend: ftuna
        }
    }
}
