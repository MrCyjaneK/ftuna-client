import QtQuick 2.6
import Sailfish.Silica 1.0
import "../components"
import "../ui.js" as Ui

Page {
    id: selectPage
    property var backend
    property string packageId
    property var rpmModel
    property var host
    property int depCount: 0

    function refreshDeps() {
        var deps = host && host.reportDeps ? host.reportDeps : []
        depCount = deps.length
    }

    Component.onCompleted: refreshDeps()

    SilicaListView {
        id: view
        anchors.fill: parent
        model: rpmModel

        header: PageHeader {
            title: "Packages"
        }

        delegate: BackgroundItem {
            id: row
            width: view.width
            height: Theme.itemSizeSmall

            function apply(on) {
                rpmModel.setProperty(index, "checked", on)
                if (host)
                    host.recount()
                selectPage.refreshDeps()
            }

            onClicked: apply(!model.checked)

            Switch {
                id: toggle
                anchors.right: parent.right
                anchors.rightMargin: Theme.horizontalPageMargin
                anchors.verticalCenter: parent.verticalCenter
                automaticCheck: false
                checked: model.checked
                onClicked: row.apply(!checked)
            }
            Label {
                anchors.left: parent.left
                anchors.leftMargin: Theme.horizontalPageMargin
                anchors.right: toggle.left
                anchors.rightMargin: Theme.paddingLarge
                anchors.verticalCenter: parent.verticalCenter
                truncationMode: TruncationMode.Fade
                color: row.highlighted ? Theme.highlightColor : Theme.primaryColor
                text: model.version ? model.name + "  " + model.version : model.name
            }
            ColorLine {
                anchors.left: parent.left
                anchors.right: toggle.left
                anchors.bottom: parent.bottom
                anchors.leftMargin: Theme.horizontalPageMargin
                anchors.rightMargin: Theme.paddingLarge
                height: Math.max(3, Theme.paddingSmall / 2)
                counts: model.line
            }
        }

        footer: BackgroundItem {
            id: depTile
            width: view.width
            height: Theme.itemSizeMedium
            onClicked: pageStack.push(Qt.resolvedUrl("DependenciesPage.qml"), {
                backend: backend,
                deps: host && host.reportDeps ? host.reportDeps : []
            })

            Label {
                id: depTitle
                x: Theme.horizontalPageMargin
                anchors.verticalCenter: parent.verticalCenter
                color: depTile.highlighted ? Theme.highlightColor : Theme.primaryColor
                text: "Dependencies"
            }
            Label {
                anchors.left: depTitle.right
                anchors.leftMargin: Theme.paddingLarge
                anchors.right: depChevron.left
                anchors.rightMargin: Theme.paddingMedium
                anchors.verticalCenter: parent.verticalCenter
                horizontalAlignment: Text.AlignRight
                truncationMode: TruncationMode.Fade
                font.pixelSize: Theme.fontSizeSmall
                color: depTile.highlighted ? Theme.secondaryHighlightColor : Theme.secondaryColor
                text: selectPage.depCount === 0 ? "None" : selectPage.depCount
            }
            Image {
                id: depChevron
                anchors.right: parent.right
                anchors.rightMargin: Theme.horizontalPageMargin
                anchors.verticalCenter: parent.verticalCenter
                source: "image://theme/icon-m-right"
            }
        }
    }
}
