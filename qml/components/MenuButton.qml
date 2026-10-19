import QtQuick 2.6
import Sailfish.Silica 1.0

BackgroundItem {
    id: button
    property alias text: label.text
    property alias busy: indicator.running

    height: Theme.itemSizeMedium

    Rectangle {
        anchors.fill: parent
        gradient: Gradient {
            GradientStop { position: 0.0; color: Theme.rgba(Theme.highlightBackgroundColor, 0.15) }
            GradientStop { position: 1.0; color: "transparent" }
        }
    }

    BusyIndicator {
        id: indicator
        anchors.left: parent.left
        anchors.leftMargin: Theme.horizontalPageMargin
        anchors.verticalCenter: parent.verticalCenter
        size: BusyIndicatorSize.Small
        running: false
    }

    Label {
        id: label
        anchors.left: parent.left
        anchors.right: chevron.left
        anchors.verticalCenter: parent.verticalCenter
        anchors.leftMargin: indicator.running ? indicator.width + Theme.horizontalPageMargin + Theme.paddingMedium : Theme.horizontalPageMargin
        anchors.rightMargin: Theme.paddingMedium
        horizontalAlignment: Text.AlignRight
        truncationMode: TruncationMode.Fade
        color: !button.enabled ? Theme.secondaryColor : button.highlighted ? Theme.highlightColor : Theme.primaryColor
    }

    Image {
        id: chevron
        visible: button.enabled
        anchors.right: parent.right
        anchors.rightMargin: Theme.horizontalPageMargin
        anchors.verticalCenter: parent.verticalCenter
        source: "image://theme/icon-m-right?" + (button.highlighted ? Theme.highlightColor : Theme.primaryColor)
    }
}
