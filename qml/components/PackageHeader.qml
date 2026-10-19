import QtQuick 2.6
import Sailfish.Silica 1.0

Item {
    id: header
    property alias title: pageHeader.title
    property alias description: pageHeader.description
    property string iconSource

    width: parent.width
    height: Math.max(column.height, icon.visible ? icon.height + icon.anchors.topMargin : 0)

    Column {
        id: column
        anchors.left: parent.left
        anchors.right: icon.visible ? icon.left : parent.right
        anchors.rightMargin: icon.visible ? Theme.paddingLarge : 0

        PageHeader {
            id: pageHeader
            width: parent.width
            rightMargin: 0
        }
    }

    Image {
        id: icon
        visible: header.iconSource !== ""
        anchors.top: parent.top
        anchors.topMargin: Theme.horizontalPageMargin + Theme.paddingMedium
        anchors.right: parent.right
        anchors.rightMargin: Theme.horizontalPageMargin
        width: Theme.iconSizeLauncher
        height: width
        source: header.iconSource
        fillMode: Image.PreserveAspectFit
    }
}
