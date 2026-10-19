import QtQuick 2.6
import Sailfish.Silica 1.0
import "../components"
import "../ui.js" as Ui

Page {
    id: page
    property var backend
    property var rows: []
    property var checked: ({})
    property int selected: 0

    function load() {
        if (!backend)
            return
        var st = backend.listing()
        var indexed = Ui.byId(st.packages || [])
        var list = Ui.idsOf(st.updates || [], indexed)
        var prev = checked
        var next = {}
        for (var i = 0; i < list.length; i++) {
            var id = list[i].id
            next[id] = prev && prev[id] === false ? false : true
        }
        rows = list
        checked = next
        recount()
    }

    function recount() {
        var n = 0
        for (var i = 0; i < rows.length; i++) {
            if (checked[rows[i].id] !== false)
                n++
        }
        selected = n
    }

    function toggle(id) {
        var next = {}
        for (var i = 0; i < rows.length; i++) {
            var key = rows[i].id
            next[key] = key === id ? checked[key] === false : checked[key] !== false
        }
        checked = next
        recount()
    }

    function setAll(on) {
        var next = {}
        for (var i = 0; i < rows.length; i++)
            next[rows[i].id] = on
        checked = next
        recount()
    }

    function chosen() {
        var out = []
        for (var i = 0; i < rows.length; i++) {
            var pkg = rows[i]
            if (checked[pkg.id] === false)
                continue
            out.push({
                id: pkg.id,
                summary: pkg.summary || pkg.id,
                icon_path: pkg.icon_path || "",
                repo: pkg.repo || "",
                color: pkg.color || "",
                installed: pkg.installed || "",
                version: pkg.version || "",
                repo_url: pkg.repo_url || "",
                specs: pkg.specs || [],
                lines: []
            })
        }
        return out
    }

    function openApply() {
        var packages = chosen()
        if (packages.length === 0)
            return
        pageStack.push(Qt.resolvedUrl("ChangePage.qml"), {
            backend: backend,
            action: "Update",
            packages: packages
        })
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

    SilicaListView {
        id: view
        anchors.fill: parent
        clip: true
        // The page sizes this list to the full screen, so the dock would cover
        // the last rows. bottomMargin lets them scroll clear of it.
        bottomMargin: dock.height
        model: page.rows
        currentIndex: -1

        header: Column {
            width: view.width

            PageHeader {
                title: "Updates"
            }
            BackgroundItem {
                width: parent.width
                height: Theme.itemSizeSmall
                visible: page.rows.length > 0
                onClicked: page.setAll(page.selected !== page.rows.length)

                Switch {
                    id: allSwitch
                    anchors.right: parent.right
                    anchors.rightMargin: Theme.horizontalPageMargin
                    anchors.verticalCenter: parent.verticalCenter
                    automaticCheck: false
                    checked: page.rows.length > 0 && page.selected === page.rows.length
                    onClicked: page.setAll(!checked)
                }
                Label {
                    anchors.left: parent.left
                    anchors.leftMargin: Theme.horizontalPageMargin
                    anchors.right: allSwitch.left
                    anchors.rightMargin: Theme.paddingMedium
                    anchors.verticalCenter: parent.verticalCenter
                    color: parent.highlighted ? Theme.highlightColor : Theme.primaryColor
                    text: page.selected + " of " + page.rows.length + " selected"
                }
            }
        }

        delegate: ListItem {
            id: row
            contentHeight: Math.max(Theme.itemSizeLarge, textColumn.height + Theme.paddingLarge)
            width: view.width
            readonly property bool on: page.checked[modelData.id] !== false
            readonly property color muted: highlighted ? Theme.secondaryHighlightColor : Theme.secondaryColor

            menu: ContextMenu {
                MenuItem {
                    text: "Details"
                    onClicked: Ui.openPackage(pageStack, backend, modelData.id, Qt.resolvedUrl("PackagePage.qml"))
                }
            }

            onClicked: page.toggle(modelData.id)

            Image {
                id: icon
                anchors.left: parent.left
                anchors.leftMargin: Theme.horizontalPageMargin
                anchors.verticalCenter: parent.verticalCenter
                width: Theme.iconSizeMedium
                height: width
                fillMode: Image.PreserveAspectFit
                source: Ui.iconSource(modelData.icon_path, "image://theme/icon-m-other")
            }
            Switch {
                id: toggle
                anchors.right: parent.right
                anchors.rightMargin: Theme.horizontalPageMargin
                anchors.verticalCenter: parent.verticalCenter
                automaticCheck: false
                checked: row.on
                onClicked: page.toggle(modelData.id)
            }
            Column {
                id: textColumn
                anchors.left: icon.right
                anchors.leftMargin: Theme.paddingMedium
                anchors.right: toggle.left
                anchors.rightMargin: Theme.paddingMedium
                anchors.verticalCenter: parent.verticalCenter
                spacing: Theme.paddingSmall

                Item {
                    width: parent.width
                    height: titleText.height + Theme.paddingSmall

                    Label {
                        id: titleText
                        anchors.left: parent.left
                        anchors.right: parent.right
                        truncationMode: TruncationMode.Fade
                        color: row.highlighted ? Theme.highlightColor : Theme.primaryColor
                        text: modelData.summary || modelData.id
                    }
                    ColorLine {
                        anchors.left: parent.left
                        anchors.right: parent.right
                        anchors.top: titleText.bottom
                        anchors.topMargin: Theme.paddingSmall / 2
                        height: Math.max(3, Theme.paddingSmall / 2)
                        counts: modelData.line
                        unknown: row.muted
                    }
                }
                Item {
                    id: versionLine
                    width: parent.width
                    height: Math.max(fromText.height, toText.height)
                    visible: fromText.text !== "" || toText.text !== ""
                    clip: true

                    Label {
                        id: fromText
                        anchors.verticalCenter: parent.verticalCenter
                        width: text === "" ? 0
                               : (toText.text === "" ? parent.width
                                  : Math.min(implicitWidth, Math.floor((parent.width - arrowText.implicitWidth - Theme.paddingMedium) / 2)))
                        font.pixelSize: Theme.fontSizeSmall
                        color: row.muted
                        truncationMode: TruncationMode.Fade
                        text: modelData.installed || ""
                    }
                    Label {
                        id: arrowText
                        anchors.left: fromText.right
                        anchors.leftMargin: fromText.text !== "" && toText.text !== "" ? Theme.paddingSmall : 0
                        anchors.verticalCenter: parent.verticalCenter
                        font.pixelSize: Theme.fontSizeSmall
                        color: row.muted
                        text: fromText.text !== "" && toText.text !== "" ? "→" : ""
                    }
                    Label {
                        id: toText
                        anchors.left: arrowText.right
                        anchors.leftMargin: text !== "" && arrowText.text !== "" ? Theme.paddingSmall : 0
                        anchors.right: parent.right
                        anchors.verticalCenter: parent.verticalCenter
                        font.pixelSize: Theme.fontSizeSmall
                        truncationMode: TruncationMode.Fade
                        color: row.highlighted ? Theme.highlightColor : Theme.primaryColor
                        text: modelData.version || ""
                    }
                }
                Label {
                    width: parent.width
                    truncationMode: TruncationMode.Fade
                    font.pixelSize: Theme.fontSizeSmall
                    color: Ui.repoColor(modelData.repo, row.muted)
                    text: modelData.repo || ""
                    visible: text !== ""
                }
            }
        }

        ViewPlaceholder {
            enabled: view.count === 0
            text: "No updates available"
        }

        VerticalScrollDecorator {}
    }

    Rectangle {
        id: dock
        z: 2
        anchors.bottom: parent.bottom
        width: parent.width
        height: page.rows.length > 0 ? updateButton.height : 0
        color: Theme.overlayBackgroundColor
        visible: page.rows.length > 0

        MenuButton {
            id: updateButton
            width: parent.width
            enabled: page.selected > 0 && !backend.busy
            text: page.selected > 0 ? "Update (" + page.selected + ")" : "Update"
            onClicked: page.openApply()
        }
    }
}
