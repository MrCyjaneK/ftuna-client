import QtQuick 2.6
import Sailfish.Silica 1.0
import "../components"
import "../ui.js" as Ui

Page {
    id: reportPage
    property var host

    SilicaFlickable {
        anchors.fill: parent
        contentHeight: column.height + Theme.paddingLarge

        Column {
            id: column
            width: parent.width
            spacing: Theme.paddingMedium

            PageHeader {
                title: host && host.sampleReport ? "Findings (sample)" : "Findings"
            }

            Repeater {
                model: host && host.reportPackages ? host.reportPackages : []

                delegate: Column {
                    id: group
                    width: column.width
                    property bool open: modelData.main === true

                    BackgroundItem {
                        width: parent.width
                        height: Theme.itemSizeSmall
                        onClicked: group.open = !group.open

                        Label {
                            anchors.left: parent.left
                            anchors.leftMargin: Theme.horizontalPageMargin
                            anchors.right: parent.right
                            anchors.rightMargin: Theme.horizontalPageMargin
                            anchors.verticalCenter: parent.verticalCenter
                            truncationMode: TruncationMode.Fade
                            color: highlighted ? Theme.highlightColor : Theme.primaryColor
                            text: (group.open ? "–  " : "+  ") + (modelData.name || "")
                        }
                        ColorLine {
                            anchors.left: parent.left
                            anchors.right: parent.right
                            anchors.bottom: parent.bottom
                            anchors.leftMargin: Theme.horizontalPageMargin
                            anchors.rightMargin: Theme.horizontalPageMargin
                            height: Math.max(3, Theme.paddingSmall / 2)
                            counts: modelData.line
                        }
                    }

                    Repeater {
                        model: group.open ? (modelData.findings || []) : []

                        delegate: Item {
                            width: column.width
                            height: finding.height + Theme.paddingSmall

                            Rectangle {
                                width: Theme.paddingSmall
                                height: parent.height
                                x: Theme.horizontalPageMargin
                                color: Ui.markColor(modelData.color, Theme.secondaryColor)
                            }
                            Column {
                                id: finding
                                x: Theme.horizontalPageMargin + Theme.paddingLarge
                                width: parent.width - x - Theme.horizontalPageMargin
                                Label {
                                    width: parent.width
                                    wrapMode: Text.Wrap
                                    color: Ui.markColor(modelData.color, Theme.secondaryColor)
                                    font.pixelSize: Theme.fontSizeMedium
                                    text: modelData.title || ""
                                }
                                Label {
                                    width: parent.width
                                    wrapMode: Text.Wrap
                                    color: Theme.primaryColor
                                    font.pixelSize: Theme.fontSizeSmall
                                    text: modelData.detail || ""
                                    visible: text !== ""
                                }
                            }
                        }
                    }
                }
            }
        }
    }
}
