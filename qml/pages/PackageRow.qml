import QtQuick 2.6
import Sailfish.Silica 1.0
import "../components"
import "../ui.js" as Ui

ListItem {
    id: row
    property var pkg

    contentHeight: Theme.itemSizeLarge

    readonly property color muted: highlighted ? Theme.secondaryHighlightColor : Theme.secondaryColor

    Image {
        id: icon
        anchors.left: parent.left
        anchors.leftMargin: Theme.horizontalPageMargin
        anchors.verticalCenter: parent.verticalCenter
        width: Theme.iconSizeLarge
        height: width
        fillMode: Image.PreserveAspectFit
        source: Ui.iconSource(pkg && pkg.icon_path ? pkg.icon_path : "",
                              pkg && pkg.category === "application"
                              ? "image://theme/icon-m-other"
                              : "image://theme/icon-m-file-rpm")
    }

    ColorLine {
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.bottom: parent.bottom
        anchors.leftMargin: Theme.horizontalPageMargin
        anchors.rightMargin: Theme.horizontalPageMargin
        height: Math.max(3, Theme.paddingSmall / 2)
        counts: pkg ? pkg.line : null
        unknown: row.muted
    }

    Column {
        anchors.left: icon.right
        anchors.leftMargin: Theme.paddingLarge
        anchors.right: parent.right
        anchors.rightMargin: Theme.horizontalPageMargin
        anchors.verticalCenter: parent.verticalCenter

        Label {
            width: parent.width
            truncationMode: TruncationMode.Fade
            color: row.highlighted ? Theme.highlightColor : Theme.primaryColor
            text: pkg && pkg.summary ? pkg.summary : (pkg ? pkg.id : "")
        }
        Row {
            width: parent.width
            spacing: Theme.paddingSmall

            Label {
                id: dateLabel
                font.pixelSize: Theme.fontSizeSmall
                color: row.muted
                text: pkg && pkg.added ? pkg.added.substring(0, 10) : ""
                visible: text !== ""
            }
            Label {
                width: Math.max(0, parent.width - dateLabel.width - (dateLabel.visible ? parent.spacing : 0))
                truncationMode: TruncationMode.Fade
                font.pixelSize: Theme.fontSizeSmall
                color: Ui.repoColor(pkg ? pkg.repo : "", row.muted)
                text: pkg && pkg.repo ? pkg.repo : ""
            }
        }
    }
}
