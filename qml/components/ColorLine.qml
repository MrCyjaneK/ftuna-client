import QtQuick 2.6
import Sailfish.Silica 1.0
import "../ui.js" as Ui

Item {
    id: bar
    property var counts
    property color unknown: Theme.secondaryColor
    implicitHeight: Math.max(3, Theme.paddingSmall / 2)

    readonly property var parts: Ui.segments(counts)

    Repeater {
        model: bar.parts
        Rectangle {
            x: modelData.x * bar.width
            width: index === bar.parts.length - 1 ? Math.max(0, bar.width - x) : modelData.w * bar.width
            height: bar.height
            color: Ui.markColor(modelData.color, bar.unknown)
        }
    }
}
