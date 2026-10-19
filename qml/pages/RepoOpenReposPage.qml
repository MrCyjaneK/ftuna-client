import QtQuick 2.6
import Sailfish.Silica 1.0

Page {
    id: page

    property var listPage
    property var backend
    property bool catalogOn: false
    property string catalogDetail: "User-published packages, may not be open source."
    property bool loading: false
    property string errorText: ""
    property string query: ""
    property var users: []

    ListModel {
        id: shown
    }

    function readCatalog() {
        var found = listPage ? listPage.openReposRow() : null
        catalogOn = found ? found.row.enabled === true : false
        if (found && found.row.detail)
            catalogDetail = found.row.detail
    }

    function rebuild() {
        var needle = query.toLowerCase()
        var rows = []
        var list = users || []
        for (var i = 0; i < list.length; i++) {
            var item = list[i] || {}
            var title = item.title || item.id || ""
            if (needle !== "" && title.toLowerCase().indexOf(needle) < 0)
                continue
            var on = item.enabled === true
            rows.push({
                title: title,
                url: item.url || "",
                on: on,
                group: on ? "Enabled" : "Not enabled"
            })
        }
        rows.sort(function(a, b) {
            if (a.on !== b.on)
                return a.on ? -1 : 1
            var at = a.title.toLowerCase()
            var bt = b.title.toLowerCase()
            if (at < bt)
                return -1
            if (at > bt)
                return 1
            return 0
        })
        shown.clear()
        for (var n = 0; n < rows.length; n++)
            shown.append(rows[n])
    }

    function load(refresh) {
        if (!backend)
            return
        loading = true
        errorText = ""
        var rows = backend.openReposUsers(refresh === true) || []
        users = rows
        loading = false
        readCatalog()
        rebuild()
    }

    Timer {
        id: resort
        interval: 1
        repeat: false
        property string repoURL
        property bool turnOn
        onTriggered: {
            if (!backend)
                return
            page.errorText = backend.setRepoEnabled(repoURL, turnOn) || ""
            page.load(false)
        }
    }

    Timer {
        id: catalogResort
        interval: 1
        repeat: false
        property bool turnOn
        onTriggered: {
            if (!listPage)
                return
            listPage.setOpenReposEnabled(turnOn)
            page.readCatalog()
            catalogSwitch.checked = page.catalogOn
            page.errorText = listPage.errorText || ""
        }
    }

    Component.onCompleted: load(false)

    Column {
        id: top
        z: 2
        width: parent.width

        PageHeader {
            title: "OpenRepos"
        }
        TextSwitch {
            id: catalogSwitch
            width: parent.width
            text: "Enabled"
            description: page.catalogDetail
            checked: page.catalogOn
            onClicked: {
                catalogResort.turnOn = checked
                catalogResort.restart()
            }
        }
        SearchField {
            id: search
            width: parent.width
            placeholderText: "Search"
            onTextChanged: {
                page.query = text
                page.rebuild()
            }
        }
        Label {
            x: Theme.horizontalPageMargin
            width: parent.width - 2 * x
            visible: page.errorText !== ""
            wrapMode: Text.Wrap
            color: "#e15b64"
            text: page.errorText
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
        model: shown
        currentIndex: -1

        PullDownMenu {
            MenuItem {
                text: "Refresh"
                onClicked: page.load(true)
            }
        }

        delegate: Item {
            id: userDelegate
            width: view.width
            height: userColumn.height
            readonly property bool showGroup: index === 0 || shown.get(index - 1).group !== model.group

            Column {
                id: userColumn
                width: parent.width
                height: implicitHeight

                Item {
                    width: parent.width
                    visible: userDelegate.showGroup
                    height: visible ? groupLabel.implicitHeight + Theme.paddingLarge : 0

                    Label {
                        id: groupLabel
                        x: Theme.horizontalPageMargin
                        y: Theme.paddingMedium
                        width: parent.width - 2 * x
                        color: Theme.highlightColor
                        font.pixelSize: Theme.fontSizeSmall
                        truncationMode: TruncationMode.Fade
                        text: model.group
                    }
                }

                BackgroundItem {
                    id: userRow
                    width: parent.width
                    height: Theme.itemSizeSmall

                    Label {
                        anchors.left: parent.left
                        anchors.leftMargin: Theme.horizontalPageMargin
                        anchors.right: userSwitch.left
                        anchors.rightMargin: Theme.paddingMedium
                        anchors.verticalCenter: parent.verticalCenter
                        truncationMode: TruncationMode.Fade
                        color: userRow.highlighted ? Theme.highlightColor : Theme.primaryColor
                        text: model.title
                    }

                    Switch {
                        id: userSwitch
                        anchors.right: parent.right
                        anchors.rightMargin: Theme.horizontalPageMargin
                        anchors.verticalCenter: parent.verticalCenter
                        checked: model.on
                        onClicked: {
                            resort.repoURL = model.url
                            resort.turnOn = checked
                            resort.restart()
                        }
                    }
                }
            }
        }

        ViewPlaceholder {
            enabled: !page.loading && shown.count === 0
            text: page.query !== "" ? "No matches" : "No publishers"
        }

        VerticalScrollDecorator {}
    }
}
