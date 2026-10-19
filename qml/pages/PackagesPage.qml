import QtQuick 2.6
import Sailfish.Silica 1.0
import "../ui.js" as Ui

Page {
    id: page
    property var backend
    property string title
    property string mode
    property string category: ""
    property string query: ""
    property var packages: []
    property var recentIds: []
    property var updateIds: []
    property var rows: []

    function load() {
        if (!backend)
            return
        var st = backend.listing()
        packages = st.packages || []
        recentIds = st.recent || []
        updateIds = st.updates || []
        refreshRows()
    }

    onQueryChanged: refreshRows()

    function listed() {
        var indexed = Ui.byId(packages)
        if (mode === "recent")
            return Ui.idsOf(recentIds, indexed)
        if (mode === "updates")
            return Ui.idsOf(updateIds, indexed)
        if (mode === "applications") {
            var apps = []
            for (var a = 0; a < packages.length; a++) {
                if (packages[a].category === "application" || packages[a].icon_path)
                    apps.push(packages[a])
            }
            return apps
        }
        if (mode === "category") {
            var matched = []
            for (var c = 0; c < packages.length; c++) {
                if (packages[c].category === category)
                    matched.push(packages[c])
            }
            return matched
        }
        return packages
    }

    function refreshRows() {
        var base = listed()
        if (query === "") {
            rows = base
            return
        }
        var needle = query.toLowerCase()
        var out = []
        for (var i = 0; i < base.length; i++) {
            var pkg = base[i]
            var hay = ((pkg.summary || "") + " " + (pkg.id || "")).toLowerCase()
            if (hay.indexOf(needle) >= 0)
                out.push(pkg)
        }
        rows = out
    }

    Connections {
        target: backend
        onRatingsReady: load()
        onRefreshed: load()
    }

    Component.onCompleted: load()

    onStatusChanged: {
        if (status === PageStatus.Active)
            load()
    }

    Column {
        id: top
        z: 2
        width: parent.width

        PageHeader {
            title: page.title
        }
        SearchField {
            id: search
            width: parent.width
            placeholderText: "Search"
            onTextChanged: page.query = text
        }
    }

    SilicaListView {
        id: view
        z: 1
        clip: true
        anchors.top: top.bottom
        anchors.bottom: parent.bottom
        anchors.left: parent.left
        anchors.right: parent.right
        model: page.rows
        currentIndex: -1

        delegate: PackageRow {
            pkg: modelData
            onClicked: Ui.openPackage(pageStack, backend, modelData.id, Qt.resolvedUrl("PackagePage.qml"))
        }

        ViewPlaceholder {
            enabled: view.count === 0
            text: "Nothing here"
        }

        VerticalScrollDecorator {}
    }
}
