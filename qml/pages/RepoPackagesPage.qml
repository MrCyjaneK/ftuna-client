import QtQuick 2.6
import Sailfish.Silica 1.0
import "../ui.js" as Ui

Page {
    id: page
    property var backend
    property string prefId
    property string title
    property bool repoOn: false
    property var rows: []

    function load() {
        if (!backend || prefId === "" || !repoOn) {
            rows = []
            return
        }
        rows = backend.repoPackages(prefId) || []
    }

    Component.onCompleted: load()

    Connections {
        target: backend
        onRefreshed: load()
    }

    onStatusChanged: {
        if (status === PageStatus.Active)
            load()
    }

    SilicaListView {
        id: view
        anchors.fill: parent
        model: page.rows
        currentIndex: -1

        header: PageHeader {
            width: view.width
            title: page.title
        }

        delegate: PackageRow {
            width: view.width
            pkg: modelData
            onClicked: Ui.openPackage(pageStack, backend, modelData.id, Qt.resolvedUrl("PackagePage.qml"))
        }

        ViewPlaceholder {
            enabled: view.count === 0
            text: page.repoOn && page.prefId !== "" ? "Nothing here" : "Turn this repository on and refresh"
        }

        VerticalScrollDecorator {}
    }
}
