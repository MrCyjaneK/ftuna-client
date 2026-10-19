import QtQuick 2.6
import Sailfish.Silica 1.0
import "../components"

Page {
    id: page
    property var backend
    property var categories: []

    function reload() {
        if (!backend)
            return
        categories = backend.listing().categories || []
    }

    Component.onCompleted: reload()

    Connections {
        target: backend
        onRefreshed: page.reload()
    }

    SilicaListView {
        id: view
        anchors.fill: parent
        model: categories

        header: PageHeader {
            title: "Categories"
        }

        delegate: MenuButton {
            width: view.width
            text: modelData.label
            onClicked: pageStack.push(Qt.resolvedUrl("PackagesPage.qml"), {
                backend: backend,
                title: modelData.label,
                mode: "category",
                category: modelData.id
            })
        }

        ViewPlaceholder {
            enabled: categories.length === 0
            text: "No categories"
        }
    }
}
