import QtQuick 2.6
import Sailfish.Silica 1.0
import "../components"
import "../ui.js" as Ui

Page {
    id: page
    property var backend
    property string shownFallback: ""
    property var packages: []
    property var recent: []
    property var updates: []
    property string errorText: ""
    property string updateError: ""

    readonly property int gridColumns: isPortrait ? 2 : 4
    readonly property int gridRows: isPortrait ? 2 : 1
    readonly property int gridLimit: gridColumns * gridRows

    function reload() {
        var st = backend.listing()
        errorText = st.error || ""
        updateError = st.update_error || ""
        packages = st.packages || []
        var indexed = Ui.byId(packages)
        recent = Ui.idsOf(st.recent, indexed)
        updates = Ui.idsOf(st.updates, indexed)
        var notes = unseenFallbacks(st.fallbacks || [])
        var key = JSON.stringify(notes)
        if (notes.length > 0 && key !== shownFallback) {
            shownFallback = key
            pageStack.push(Qt.resolvedUrl("WarningPage.qml"), { backend: backend, notes: notes })
        }
    }

    function sameFallback(a, b) {
        return a.title === b.title
                && a.device_release === b.device_release
                && a.chosen_release === b.chosen_release
                && a.arch === b.arch
    }

    function unseenFallbacks(notes) {
        var seen = backend.settings().seen_fallbacks || []
        var out = []
        for (var i = 0; i < notes.length; i++) {
            var known = false
            for (var j = 0; j < seen.length; j++) {
                if (sameFallback(seen[j], notes[i])) {
                    known = true
                    break
                }
            }
            if (!known)
                out.push(notes[i])
        }
        return out
    }

    function openList(title, mode) {
        pageStack.push(Qt.resolvedUrl("PackagesPage.qml"), {
            backend: backend,
            title: title,
            mode: mode
        })
    }

    Component.onCompleted: backend.refresh()

    Connections {
        target: backend
        onRefreshed: reload()
        onRatingsReady: reload()
    }

    SilicaFlickable {
        anchors.fill: parent
        contentHeight: column.height

        PullDownMenu {
            busy: backend.busy
            MenuItem {
                text: "Refresh"
                enabled: !backend.busy
                onClicked: backend.refresh()
            }
            MenuItem {
                text: "Repositories"
                onClicked: pageStack.push(Qt.resolvedUrl("RepoPage.qml"), { backend: backend, setup: false })
            }
            MenuItem {
                text: "Search"
                onClicked: page.openList("Packages", "all")
            }
            MenuItem {
                text: "About"
                onClicked: pageStack.push(Qt.resolvedUrl("AboutPage.qml"), { backend: backend })
            }
        }

        Column {
            id: column
            width: parent.width

            PageHeader {
                title: "FTuna"
            }
            ProgressBar {
                width: parent.width
                visible: backend.busy || backend.activity !== ""
                value: backend.progress / 100.0
            }
            Label {
                x: Theme.horizontalPageMargin
                width: parent.width - 2 * x
                visible: backend.busy || backend.activity !== ""
                wrapMode: Text.Wrap
                color: Theme.highlightColor
                font.pixelSize: Theme.fontSizeSmall
                text: backend.activity !== "" ? backend.activity : "Refreshing"
            }
            Label {
                x: Theme.horizontalPageMargin
                width: parent.width - 2 * x
                visible: backend.notice !== ""
                wrapMode: Text.Wrap
                color: Theme.highlightColor
                text: backend.notice
            }
            Label {
                x: Theme.horizontalPageMargin
                width: parent.width - 2 * x
                visible: errorText !== "" && !backend.busy
                wrapMode: Text.Wrap
                color: Theme.secondaryColor
                text: errorText
            }

            MenuButton {
                text: recent.length ? "Recently added" : "Nothing added yet"
                enabled: recent.length > 0
                busy: backend.busy && recent.length === 0
                onClicked: page.openList("Recently added", "recent")
            }

            Grid {
                width: parent.width
                columns: page.gridColumns
                visible: recent.length > 0

                Repeater {
                    model: Math.min(recent.length, page.gridLimit)
                    delegate: BackgroundItem {
                        id: tile
                        width: column.width / page.gridColumns
                        height: Theme.itemSizeLarge
                        readonly property color muted: highlighted ? Theme.secondaryHighlightColor : Theme.secondaryColor
                        onClicked: Ui.openPackage(pageStack, backend, recent[index].id, Qt.resolvedUrl("PackagePage.qml"))

                        Image {
                            id: icon
                            anchors.left: parent.left
                            anchors.leftMargin: Theme.horizontalPageMargin
                            anchors.verticalCenter: parent.verticalCenter
                            width: Theme.iconSizeMedium
                            height: width
                            fillMode: Image.PreserveAspectFit
                            source: Ui.iconSource(recent[index].icon_path, "image://theme/icon-m-other")
                        }
                        ColorLine {
                            anchors.left: parent.left
                            anchors.right: parent.right
                            anchors.bottom: parent.bottom
                            anchors.leftMargin: Theme.horizontalPageMargin
                            anchors.rightMargin: Theme.horizontalPageMargin
                            height: Math.max(3, Theme.paddingSmall / 2)
                            counts: recent[index].line
                            unknown: tile.muted
                        }
                        Column {
                            anchors.left: icon.right
                            anchors.leftMargin: Theme.paddingMedium
                            anchors.right: parent.right
                            anchors.rightMargin: Theme.horizontalPageMargin
                            anchors.verticalCenter: parent.verticalCenter

                            Label {
                                width: parent.width
                                truncationMode: TruncationMode.Fade
                                color: tile.highlighted ? Theme.highlightColor : Theme.primaryColor
                                text: recent[index].summary || recent[index].id
                            }
                            Label {
                                width: parent.width
                                truncationMode: TruncationMode.Fade
                                font.pixelSize: Theme.fontSizeSmall
                                color: Ui.repoColor(recent[index].repo, tile.muted)
                                text: recent[index].repo || ""
                                visible: text !== ""
                            }
                        }
                    }
                }
            }

            MenuButton {
                text: updates.length ? ("Updates (" + updates.length + ")") : "No updates available"
                enabled: updates.length > 0 && updateError === ""
                onClicked: pageStack.push(Qt.resolvedUrl("UpdatesPage.qml"), { backend: backend })
            }
            Label {
                x: Theme.horizontalPageMargin
                width: parent.width - 2 * x
                visible: updateError !== ""
                wrapMode: Text.Wrap
                color: Theme.secondaryColor
                font.pixelSize: Theme.fontSizeSmall
                text: updateError
            }

            MenuButton {
                text: "Categories"
                onClicked: pageStack.push(Qt.resolvedUrl("CategoriesPage.qml"), { backend: backend })
            }
            MenuButton {
                text: "Applications"
                onClicked: page.openList("Applications", "applications")
            }
        }
    }
}
