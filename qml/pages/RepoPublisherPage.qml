import QtQuick 2.6
import Sailfish.Silica 1.0

Page {
    id: page

    property var listPage
    property string publisher
    property string alias: ""
    property bool loading: false
    property string errorText: ""
    property var remote: []
    property bool alive: true
    property var request

    ListModel {
        id: pads
    }

    function note(expire) {
        if (!expire)
            return ""
        var left = new Date(expire * 1000).getTime() - Date.now()
        if (left <= 0)
            return "Expired"
        var days = Math.ceil(left / 86400000)
        return days === 1 ? "1 day left" : (days + " days left")
    }

    function refreshAlias() {
        alias = listPage && listPage.publisherName(publisher) ? listPage.publisherName(publisher) : ""
    }

    function rebuild() {
        refreshAlias()
        var local = {}
        var known = listPage ? listPage.scratchpadsFor(publisher) : []
        for (var i = 0; i < known.length; i++)
            local[known[i].repoId] = known[i]
        var seen = {}
        var rows = []
        var published = remote || []
        for (var r = 0; r < published.length; r++) {
            var item = published[r]
            var id = item && item.id ? item.id : ""
            if (id === "" || id.indexOf("/") >= 0 || seen[id])
                continue
            seen[id] = true
            var got = local[id]
            var turnedOn = got ? got.enabled === true : false
            rows.push({
                repoId: id,
                prefId: got ? (got.prefId || "") : "",
                title: got ? got.title : id,
                named: got ? got.named === true : false,
                stored: !!got,
                on: turnedOn,
                group: turnedOn ? "Enabled" : "Not enabled",
                created: item.created || 0,
                note: page.note(item.expire)
            })
        }
        for (var repoId in local) {
            if (seen[repoId])
                continue
            var extra = local[repoId]
            rows.push({
                repoId: extra.repoId,
                prefId: extra.prefId || "",
                title: extra.title,
                named: extra.named === true,
                stored: true,
                on: extra.enabled === true,
                group: extra.enabled === true ? "Enabled" : "Not enabled",
                created: 0,
                note: "Not published anymore"
            })
        }
        rows.sort(function(a, b) {
            if (a.on !== b.on)
                return a.on ? -1 : 1
            if (a.created !== b.created)
                return b.created - a.created
            if (a.title < b.title)
                return -1
            if (a.title > b.title)
                return 1
            return 0
        })
        pads.clear()
        for (var n = 0; n < rows.length; n++)
            pads.append(rows[n])
    }

    function fetch() {
        if (!/^[A-Za-z0-9._-]+$/.test(publisher)) {
            errorText = "Could not load this publisher"
            return
        }
        loading = true
        errorText = ""
        var xhr = new XMLHttpRequest()
        request = xhr
        xhr.onreadystatechange = function() {
            if (xhr.readyState !== XMLHttpRequest.DONE || !alive)
                return
            loading = false
            if (xhr.status === 200) {
                try {
                    var doc = JSON.parse(xhr.responseText)
                    remote = doc && doc.repos ? doc.repos : []
                    errorText = ""
                } catch (e) {
                    remote = []
                    errorText = "Could not read this publisher"
                }
            } else {
                remote = []
                errorText = "Could not load this publisher"
            }
            rebuild()
        }
        xhr.open("GET", "https://scratchpad-repo.ftuna.eu/" + encodeURIComponent(publisher) + "/user.json")
        xhr.send()
    }

    Timer {
        id: resort
        interval: 1
        repeat: false
        property string repoId
        property bool turnOn
        onTriggered: {
            if (!alive || !listPage)
                return
            listPage.setScratchpadEnabled(page.publisher, repoId, turnOn)
            page.rebuild()
        }
    }

    Component.onCompleted: {
        rebuild()
        fetch()
    }
    Component.onDestruction: alive = false

    Component {
        id: publisherDialog
        Dialog {
            id: publisherEdit
            property string editName: ""
            canAccept: true
            onAccepted: {
                if (!listPage)
                    return
                listPage.setPublisherName(page.publisher, nameField.text)
                page.refreshAlias()
            }

            SilicaFlickable {
                anchors.fill: parent
                contentHeight: column.height

                Column {
                    id: column
                    width: parent.width

                    DialogHeader {
                        title: "Publisher"
                        acceptText: "Save"
                    }
                    TextField {
                        id: nameField
                        width: parent.width
                        text: editName
                        label: "Name"
                        placeholderText: page.publisher
                        EnterKey.enabled: true
                        EnterKey.iconSource: "image://theme/icon-m-enter-accept"
                        EnterKey.onClicked: publisherEdit.accept()
                    }
                }
            }
        }
    }

    Component {
        id: padDialog
        Dialog {
            id: padEdit
            property string repoId: ""
            property string editName: ""
            property string prefId: ""
            property bool repoOn: false
            property bool stored: false
            canAccept: true
            onAccepted: {
                if (!listPage)
                    return
                listPage.setScratchpadTitle(page.publisher, repoId, nameField.text)
                page.rebuild()
            }

            SilicaFlickable {
                anchors.fill: parent
                contentHeight: column.height

                Column {
                    id: column
                    width: parent.width

                    DialogHeader {
                        title: "Scratchpad"
                        acceptText: "Save"
                    }
                    Label {
                        x: Theme.horizontalPageMargin
                        width: parent.width - 2 * x
                        wrapMode: Text.Wrap
                        color: Theme.secondaryColor
                        font.pixelSize: Theme.fontSizeSmall
                        text: padEdit.repoId
                    }
                    TextField {
                        id: nameField
                        width: parent.width
                        text: editName
                        label: "Name"
                        placeholderText: padEdit.repoId
                        EnterKey.enabled: true
                        EnterKey.iconSource: "image://theme/icon-m-enter-accept"
                        EnterKey.onClicked: padEdit.accept()
                    }
                    BackgroundItem {
                        width: parent.width
                        height: Theme.itemSizeSmall
                        onClicked: pageStack.push(Qt.resolvedUrl("RepoPackagesPage.qml"), {
                            backend: listPage ? listPage.backend : null,
                            prefId: padEdit.prefId,
                            title: nameField.text || padEdit.editName || padEdit.repoId,
                            repoOn: padEdit.repoOn
                        })

                        Label {
                            anchors.left: parent.left
                            anchors.leftMargin: Theme.horizontalPageMargin
                            anchors.right: packagesChevron.left
                            anchors.rightMargin: Theme.paddingMedium
                            anchors.verticalCenter: parent.verticalCenter
                            truncationMode: TruncationMode.Fade
                            color: parent.highlighted ? Theme.highlightColor : Theme.primaryColor
                            text: "Packages"
                        }
                        Image {
                            id: packagesChevron
                            anchors.right: parent.right
                            anchors.rightMargin: Theme.horizontalPageMargin
                            anchors.verticalCenter: parent.verticalCenter
                            source: "image://theme/icon-m-right?" + (parent.highlighted ? Theme.highlightColor : Theme.primaryColor)
                        }
                    }
                    BackgroundItem {
                        width: parent.width
                        height: visible ? Theme.itemSizeSmall : 0
                        visible: padEdit.stored
                        onClicked: remorse.execute("Removing", function() {
                            if (listPage)
                                listPage.removeScratchpad(page.publisher, padEdit.repoId)
                            padEdit.reject()
                            page.rebuild()
                        })

                        Label {
                            anchors.centerIn: parent
                            color: parent.highlighted ? Theme.highlightColor : "#e15b64"
                            text: "Remove"
                        }
                    }
                }
            }

            RemorsePopup { id: remorse }
        }
    }

    function openPad(repoId, title, named, stored, prefId, repoOn) {
        pageStack.push(padDialog, {
            repoId: repoId,
            editName: named ? title : "",
            stored: stored,
            prefId: prefId || "",
            repoOn: repoOn === true
        })
    }

    SilicaListView {
        id: view
        anchors.fill: parent
        model: pads
        currentIndex: -1

        PullDownMenu {
            MenuItem {
                text: "Rename publisher"
                onClicked: pageStack.push(publisherDialog, { editName: page.alias })
            }
        }

        header: Column {
            width: view.width
            height: implicitHeight

            PageHeader {
                width: parent.width
                title: page.alias !== "" ? page.alias : "Scratchpads"
                description: page.publisher
            }
            Item {
                width: parent.width
                visible: page.loading
                height: visible ? busy.height + Theme.paddingMedium : 0

                BusyIndicator {
                    id: busy
                    anchors.horizontalCenter: parent.horizontalCenter
                    running: page.loading
                    size: BusyIndicatorSize.Small
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

        delegate: Item {
            id: padDelegate
            width: view.width
            height: padColumn.height
            readonly property bool showGroup: index === 0 || pads.get(index - 1).group !== model.group

            Column {
                id: padColumn
                width: parent.width
                height: implicitHeight

                Item {
                    width: parent.width
                    visible: padDelegate.showGroup
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
            id: padRow
            width: view.width
            height: Theme.itemSizeLarge
            onClicked: page.openPad(model.repoId, model.title, model.named, model.stored, model.prefId, model.on)

            Column {
                anchors.left: parent.left
                anchors.leftMargin: Theme.horizontalPageMargin
                anchors.right: padSwitch.left
                anchors.rightMargin: Theme.paddingMedium
                anchors.verticalCenter: parent.verticalCenter

                Label {
                    width: parent.width
                    truncationMode: TruncationMode.Fade
                    color: padRow.highlighted ? Theme.highlightColor : Theme.primaryColor
                    text: model.title
                }
                Label {
                    width: parent.width
                    visible: text !== ""
                    truncationMode: TruncationMode.Fade
                    font.pixelSize: Theme.fontSizeSmall
                    color: padRow.highlighted ? Theme.secondaryHighlightColor : Theme.secondaryColor
                    text: model.note
                }
            }

            Switch {
                id: padSwitch
                anchors.right: parent.right
                anchors.rightMargin: Theme.horizontalPageMargin
                anchors.verticalCenter: parent.verticalCenter
                checked: model.on
                onClicked: {
                    resort.repoId = model.repoId
                    resort.turnOn = checked
                    resort.restart()
                }
            }
                }
            }
        }

        ViewPlaceholder {
            enabled: !page.loading && pads.count === 0
            text: "No scratchpads"
        }

        VerticalScrollDecorator {}
    }
}
