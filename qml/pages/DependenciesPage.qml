import QtQuick 2.6
import Sailfish.Silica 1.0
import "../components"
import "../ui.js" as Ui

Page {
    id: depPage
    property var backend
    property var deps: []

    SilicaListView {
        anchors.fill: parent
        model: deps

        header: PageHeader {
            title: "Dependencies"
        }

        ViewPlaceholder {
            enabled: deps.length === 0
            text: "None listed"
        }

        delegate: BackgroundItem {
            id: row
            width: parent.width
            height: Theme.itemSizeSmall
            enabled: modelData.known === true && (modelData.id || "") !== ""
            onClicked: Ui.openPackage(pageStack, backend, modelData.id, Qt.resolvedUrl("PackagePage.qml"))

            Label {
                anchors.left: parent.left
                anchors.leftMargin: Theme.horizontalPageMargin
                anchors.right: chevron.left
                anchors.rightMargin: Theme.paddingMedium
                anchors.verticalCenter: parent.verticalCenter
                truncationMode: TruncationMode.Fade
                color: row.highlighted ? Theme.highlightColor : Theme.primaryColor
                text: modelData.name
            }
            ColorLine {
                anchors.left: parent.left
                anchors.right: chevron.left
                anchors.bottom: parent.bottom
                anchors.leftMargin: Theme.horizontalPageMargin
                anchors.rightMargin: Theme.paddingMedium
                height: Math.max(3, Theme.paddingSmall / 2)
                visible: modelData.known === true
                counts: modelData.line
            }
            Image {
                id: chevron
                anchors.right: parent.right
                anchors.rightMargin: Theme.horizontalPageMargin
                anchors.verticalCenter: parent.verticalCenter
                visible: row.enabled
                width: visible ? implicitWidth : 0
                source: "image://theme/icon-m-right"
            }
        }
    }
}
