import QtQuick 2.6
import Sailfish.Silica 1.0

Page {
    id: page

    property var backend
    property string errorText: ""

    SilicaFlickable {
        anchors.fill: parent
        anchors.bottomMargin: dock.height
        contentHeight: column.height

        Column {
            id: column
            width: parent.width
            spacing: Theme.paddingLarge

            PageHeader {
                title: "About"
            }

            Image {
                anchors.horizontalCenter: parent.horizontalCenter
                width: Theme.iconSizeLauncher
                height: width
                source: Qt.resolvedUrl("../images/ftuna.png")
                fillMode: Image.PreserveAspectFit
                smooth: true
                mipmap: true
            }

            Label {
                anchors.horizontalCenter: parent.horizontalCenter
                width: parent.width - 2 * Theme.horizontalPageMargin
                horizontalAlignment: Text.AlignHCenter
                font.pixelSize: Theme.fontSizeLarge
                color: Theme.highlightColor
                text: "FTuna"
            }

            Column {
                width: parent.width
                spacing: Theme.paddingSmall

                Label {
                    anchors.horizontalCenter: parent.horizontalCenter
                    width: parent.width - 2 * Theme.horizontalPageMargin
                    horizontalAlignment: Text.AlignHCenter
                    font.pixelSize: Theme.fontSizeSmall
                    color: Theme.secondaryColor
                    text: "Author"
                }
                Label {
                    anchors.horizontalCenter: parent.horizontalCenter
                    width: parent.width - 2 * Theme.horizontalPageMargin
                    horizontalAlignment: Text.AlignHCenter
                    wrapMode: Text.Wrap
                    text: "Czarek Nakamoto"
                }
            }
        }
    }

    Column {
        id: dock
        width: parent.width
        anchors.bottom: parent.bottom
        anchors.bottomMargin: Theme.paddingLarge
        spacing: Theme.paddingMedium

        Label {
            x: Theme.horizontalPageMargin
            width: parent.width - 2 * x
            visible: page.errorText !== ""
            wrapMode: Text.Wrap
            horizontalAlignment: Text.AlignHCenter
            color: "#e15b64"
            text: page.errorText
        }

        Button {
            anchors.horizontalCenter: parent.horizontalCenter
            preferredWidth: Theme.buttonWidthLarge
            text: "Wipe all data and exit"
            onClicked: remorse.execute("Wiping data", function() {
                var err = page.backend.wipe()
                if (err !== "")
                    page.errorText = err
            })
        }
    }

    RemorsePopup { id: remorse }
}
