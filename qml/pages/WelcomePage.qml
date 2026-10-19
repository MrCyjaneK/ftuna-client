import QtQuick 2.6
import Sailfish.Silica 1.0

Dialog {
    id: page

    property var backend
    property int step: 0

    readonly property var copy: [
        "FTuna is a catalog of Sailfish software catalogs. You can access Chum, OpenRepos and any other catalog you want!",
        "FTuna-OSS is FTuna's own catalog. It contains open source software built offline directly from Git checkout, all recipes are hosted on GitHub https://github.com/MrCyjaneK/ftuna-recipes.",
        "FTuna can also be used to add scratchpad repositories, when you want to release RPM to the public but it's not quite ready yet - you can publish it and get a proper RPM repo hosted!"
    ]

    canAccept: true
    acceptDestination: page.step < 3 ? Qt.resolvedUrl("WelcomePage.qml") : Qt.resolvedUrl("RepoPage.qml")
    acceptDestinationProperties: page.step < 3
            ? { "backend": page.backend, "step": page.step + 1 }
            : { "backend": page.backend, "setup": true }

    SilicaFlickable {
        anchors.fill: parent
        contentHeight: column.height + Theme.paddingLarge

        Column {
            id: column
            width: parent.width
            spacing: Theme.paddingLarge

            DialogHeader {
                acceptText: page.step < 3 ? "Next" : "Choose repositories"
                cancelText: page.step === 0 ? "" : "Previous"
            }

            Image {
                x: Math.round((parent.width - width) / 2)
                width: Theme.iconSizeLauncher
                height: width
                source: Qt.resolvedUrl("../images/ftuna.png")
                fillMode: Image.PreserveAspectFit
                smooth: true
                mipmap: true
            }
            Label {
                x: Theme.horizontalPageMargin
                width: parent.width - 2 * x
                wrapMode: Text.Wrap
                font.pixelSize: page.step === 0 ? Theme.fontSizeExtraLarge : Theme.fontSizeMedium
                text: page.step === 0 ? "Hey This is FTuna" : page.copy[page.step - 1]
            }
        }
    }
}
