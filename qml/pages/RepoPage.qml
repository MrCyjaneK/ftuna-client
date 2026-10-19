import QtQuick 2.6
import Sailfish.Silica 1.0
import "../ui.js" as Ui

Page {
    id: page
    property var backend
    property bool setup: false
    property bool dirty: false
    property string errorText: ""
    property int openedDepth: 0
    property var publisherNames: ({})

    readonly property string scratchpadHost: "scratchpad-repo.ftuna.eu"

    ListModel {
        id: repos
    }
    ListModel {
        id: shown
    }

    function builtin(id) {
        return id === "oss" || id === "chum" || id === "openrepos"
    }

    function scratchpadParts(url) {
        var prefix = "https://" + scratchpadHost + "/"
        if (!url || url.indexOf(prefix) !== 0)
            return null
        var bits = url.substring(prefix.length).split("/")
        var parts = []
        for (var i = 0; i < bits.length; i++) {
            if (bits[i] !== "")
                parts.push(bits[i])
        }
        if (parts.length !== 2)
            return null
        return { publisher: parts[0], repo: parts[1] }
    }

    function scratchpadURL(publisher, repo) {
        return "https://" + scratchpadHost + "/" + publisher + "/" + repo + "/"
    }

    function groupOf(id, url) {
        if (builtin(id))
            return "Catalogs"
        if (scratchpadParts(url))
            return "Scratchpad"
        return "Added"
    }

    function dotColor(url, color) {
        if (scratchpadParts(url))
            return "#64b5f6"
        if (color === "orange")
            return "#ff9800"
        return "#2e7d32"
    }

    function subtitle(url, detail, problem, enabled) {
        var line = problem || ""
        if (!line) {
            var parts = scratchpadParts(url)
            line = parts ? (parts.publisher + " / " + parts.repo) : (detail || "")
        }
        if (enabled)
            return line
        return line ? ("Off · " + line) : "Off"
    }

    function catalogInfo(id, rows) {
        rows = rows || []
        for (var i = 0; i < rows.length; i++) {
            if (rows[i].id !== id)
                continue
            if (rows[i].error)
                return { text: rows[i].error, problem: rows[i].error }
            if (rows[i].fallback) {
                var note = "This phone is " + rows[i].device_release + " " + rows[i].arch + ". Using " + rows[i].chosen_release + " at " + rows[i].url
                return { text: note, problem: note }
            }
            if (rows[i].url)
                return { text: rows[i].chosen_release + " " + rows[i].arch + " at " + rows[i].url, problem: "" }
        }
        return { text: "", problem: "" }
    }

    function addRow(repo, catalogs) {
        var url = repo.url || ""
        var info = catalogInfo(repo.id, catalogs)
        repos.append({
            id: repo.id || "",
            host: repo.host || "",
            url: url,
            enabled: repo.enabled === true,
            color: repo.color || "green",
            title: repo.title || repo.host || "Repository",
            detail: repo.detail || "",
            builtin: builtin(repo.id),
            status: info.text,
            problem: info.problem,
            group: groupOf(repo.id, url)
        })
    }

    function loadSettings() {
        if (!backend)
            return
        var catalogs = backend.listing().catalogs || []
        repos.clear()
        var settings = backend.settings()
        var list = settings.repositories || []
        for (var i = 0; i < list.length; i++)
            addRow(list[i], catalogs)
        readPublisherNames(settings)
        rebuildShown()
    }

    function readPublisherNames(settings) {
        var next = {}
        var names = settings.publisher_names || []
        for (var i = 0; i < names.length; i++) {
            var id = names[i].id || ""
            var title = names[i].title || ""
            if (id !== "" && title !== "")
                next[id] = title
        }
        publisherNames = next
    }

    function publisherName(id) {
        var names = publisherNames || {}
        return names[id] || ""
    }

    function publisherLine(on) {
        if (on <= 0)
            return "Off"
        return on === 1 ? "On" : (on + " on")
    }

    function rebuildShown() {
        var catalogs = []
        var pubs = {}
        var pubOrder = []
        var added = []
        for (var i = 0; i < repos.count; i++) {
            var row = repos.get(i)
            var parts = scratchpadParts(row.url)
            if (row.builtin) {
                catalogs.push(viewRepo(row, i))
                continue
            }
            if (parts) {
                if (!pubs[parts.publisher]) {
                    pubs[parts.publisher] = { publisher: parts.publisher, on: 0 }
                    pubOrder.push(parts.publisher)
                }
                if (row.enabled)
                    pubs[parts.publisher].on++
                continue
            }
            added.push(viewRepo(row, i))
        }
        shown.clear()
        for (var c = 0; c < catalogs.length; c++)
            shown.append(catalogs[c])
        for (var p = 0; p < pubOrder.length; p++) {
            var pub = pubs[pubOrder[p]]
            shown.append({
                kind: "publisher",
                repoIndex: -1,
                publisher: pub.publisher,
                title: publisherName(pub.publisher) || pub.publisher,
                url: "",
                on: pub.on > 0,
                color: "blue",
                line: publisherLine(pub.on),
                problem: "",
                group: "Scratchpad"
            })
        }
        for (var a = 0; a < added.length; a++)
            shown.append(added[a])
    }

    function viewRepo(row, index) {
        return {
            kind: "repo",
            repoIndex: index,
            publisher: "",
            title: row.title || "",
            url: row.url || "",
            on: row.enabled === true,
            color: row.color || "green",
            line: subtitle(row.url, row.detail, row.problem, row.enabled === true),
            problem: row.problem || "",
            group: row.builtin ? "Catalogs" : "Added"
        }
    }

    function scratchpadsFor(publisher) {
        var out = []
        for (var i = 0; i < repos.count; i++) {
            var row = repos.get(i)
            var parts = scratchpadParts(row.url)
            if (!parts || parts.publisher !== publisher)
                continue
            var title = row.title || ""
            if (title === "" || title === scratchpadHost || title === parts.repo)
                title = parts.repo
            out.push({
                repoId: parts.repo,
                prefId: row.id || "",
                title: title,
                named: title !== parts.repo,
                stored: true,
                enabled: row.enabled === true
            })
        }
        return out
    }

    function findScratchpad(publisher, repoId) {
        for (var i = 0; i < repos.count; i++) {
            var parts = scratchpadParts(repos.get(i).url)
            if (parts && parts.publisher === publisher && parts.repo === repoId)
                return i
        }
        return -1
    }

    function setScratchpadEnabled(publisher, repoId, on) {
        var index = findScratchpad(publisher, repoId)
        if (index >= 0) {
            var prev = repos.get(index)
            repos.set(index, {
                id: prev.id,
                host: prev.host,
                url: prev.url,
                enabled: on === true,
                color: "blue",
                title: prev.title || repoId,
                detail: prev.detail || "Added on this phone.",
                builtin: false,
                status: prev.status || "",
                problem: prev.problem || "",
                group: "Scratchpad"
            })
        } else if (on !== true) {
            return
        } else {
            repos.append({
                id: "custom-" + scratchpadHost + "-" + publisher + "-" + repoId,
                host: scratchpadHost,
                url: scratchpadURL(publisher, repoId),
                enabled: true,
                color: "blue",
                title: repoId,
                detail: "Added on this phone.",
                builtin: false,
                status: "",
                problem: "",
                group: "Scratchpad"
            })
        }
        persist()
    }

    function scratchpadRow(prev, publisher, repoId, enabled, title) {
        return {
            id: prev ? prev.id : ("custom-" + scratchpadHost + "-" + publisher + "-" + repoId),
            host: prev ? prev.host : scratchpadHost,
            url: prev ? prev.url : scratchpadURL(publisher, repoId),
            enabled: enabled === true,
            color: "blue",
            title: title || repoId,
            detail: prev && prev.detail ? prev.detail : "Added on this phone.",
            builtin: false,
            status: prev && prev.status ? prev.status : "",
            problem: prev && prev.problem ? prev.problem : "",
            group: "Scratchpad"
        }
    }

    function setScratchpadTitle(publisher, repoId, title) {
        var name = Ui.trim(title)
        if (name === "")
            name = repoId
        var index = findScratchpad(publisher, repoId)
        if (index >= 0) {
            repos.set(index, scratchpadRow(repos.get(index), publisher, repoId, repos.get(index).enabled === true, name))
        } else if (name === repoId) {
            return
        } else {
            repos.append(scratchpadRow(null, publisher, repoId, false, name))
        }
        persist()
    }

    function removeScratchpad(publisher, repoId) {
        var index = findScratchpad(publisher, repoId)
        if (index < 0)
            return
        repos.remove(index)
        persist()
    }

    function setPublisherName(publisher, title) {
        var name = Ui.trim(title)
        var next = {}
        var names = publisherNames || {}
        for (var id in names) {
            if (names[id])
                next[id] = names[id]
        }
        if (name === "" || name === publisher)
            delete next[publisher]
        else
            next[publisher] = name
        publisherNames = next
        persist()
    }

    Component.onCompleted: loadSettings()
    onBackendChanged: loadSettings()

    Connections {
        target: backend
        onRefreshed: if (!page.dirty) page.loadSettings()
    }

    function applyRepo(editIndex, fields) {
        var url = Ui.trim(fields.url).replace(/info\.json\.xz$/, "")
        var match = url.match(/^https:\/\/([^\/]+)/)
        if (!match)
            return
        if (url.charAt(url.length - 1) !== "/")
            url += "/"
        var host = match[1]
        var prev = editIndex >= 0 ? repos.get(editIndex) : null
        var title = Ui.trim(fields.title)
        if (title === "")
            title = prev && prev.title ? prev.title : host
        var scratch = scratchpadParts(url) !== null
        var row = {
            id: prev ? prev.id : ("custom-" + host),
            host: host,
            url: url,
            enabled: fields.enabled === true,
            color: scratch ? "blue" : (prev && prev.color ? prev.color : "green"),
            title: title,
            detail: prev && prev.detail ? prev.detail : "Added on this phone.",
            builtin: prev ? prev.builtin === true : false,
            status: prev && prev.status ? prev.status : "",
            problem: prev && prev.problem ? prev.problem : "",
            group: groupOf(prev ? prev.id : "", url)
        }
        if (editIndex >= 0)
            repos.set(editIndex, row)
        else
            repos.append(row)
        persist()
    }

    function removeRepo(index) {
        if (index < 0 || index >= repos.count)
            return
        if (repos.get(index).builtin)
            return
        repos.remove(index)
        persist()
    }

    function snapshot() {
        var current = backend.settings()
        var body = {
            onboarded: current.onboarded === true,
            repositories: [],
            publisher_names: publisherNameList()
        }
        for (var i = 0; i < repos.count; i++) {
            var row = repos.get(i)
            body.repositories.push({
                id: row.id,
                host: row.host,
                url: row.url,
                enabled: row.enabled === true,
                color: row.color || "green",
                title: row.title || "",
                detail: row.detail || ""
            })
        }
        return body
    }

    function publisherNameList() {
        var out = []
        var names = publisherNames || {}
        for (var id in names) {
            if (names[id])
                out.push({ id: id, title: names[id] })
        }
        return out
    }

    function persist() {
        if (!backend)
            return
        var err = backend.saveSettings(JSON.stringify(snapshot()))
        errorText = err || ""
        if (!err) {
            dirty = true
            loadSettings()
        }
    }

    function openReposRow() {
        for (var i = 0; i < repos.count; i++) {
            if (repos.get(i).id === "openrepos")
                return { index: i, row: repos.get(i) }
        }
        return null
    }

    function setOpenReposEnabled(on) {
        var found = openReposRow()
        if (!found)
            return
        applyRepo(found.index, {
            title: found.row.title || "",
            url: found.row.url || "",
            enabled: on === true
        })
    }

    function openRow(index) {
        var row = shown.get(index)
        if (row.kind === "publisher") {
            pageStack.push(Qt.resolvedUrl("RepoPublisherPage.qml"), {
                listPage: page,
                publisher: row.publisher
            })
            return
        }
        var repo = repos.get(row.repoIndex)
        if (repo.id === "openrepos") {
            pageStack.push(Qt.resolvedUrl("RepoOpenReposPage.qml"), {
                listPage: page,
                backend: backend
            })
            return
        }
        pageStack.push(Qt.resolvedUrl("RepoEditPage.qml"), {
            listPage: page,
            editIndex: row.repoIndex,
            builtin: repo.builtin === true,
            editName: repo.title || "",
            editURL: repo.url || "",
            editEnabled: repo.enabled === true,
            editDetail: repo.builtin ? (repo.detail || "") : "",
            editStatus: repo.status || "",
            editId: repo.id || ""
        })
    }

    function openCreate() {
        pageStack.push(Qt.resolvedUrl("RepoEditPage.qml"), {
            listPage: page,
            editIndex: -1,
            builtin: false,
            editEnabled: true
        })
    }

    function openMain() {
        while (pageStack.depth > 1)
            pageStack.pop(undefined, PageStackAction.Immediate)
        pageStack.replace(Qt.resolvedUrl("MainPage.qml"), { backend: backend }, PageStackAction.Immediate)
    }

    function finishSetup() {
        if (!backend)
            return
        var body = snapshot()
        body.onboarded = true
        var err = backend.saveSettings(JSON.stringify(body))
        errorText = err || ""
        if (err)
            return
        dirty = false
        openMain()
    }

    onStatusChanged: {
        if (status === PageStatus.Active)
            openedDepth = pageStack.depth
        else if (status === PageStatus.Inactive && dirty && pageStack.depth < openedDepth) {
            dirty = false
            if (backend)
                backend.refresh()
        }
    }

    SilicaListView {
        id: reposView
        anchors.fill: parent
        model: shown
        currentIndex: -1

        PullDownMenu {
            MenuItem {
                text: "Add repository"
                onClicked: page.openCreate()
            }
            MenuItem {
                text: "Done"
                visible: page.setup
                onClicked: page.finishSetup()
            }
        }

        header: Column {
            width: reposView.width
            height: implicitHeight

            PageHeader {
                width: parent.width
                title: "Repositories"
                description: page.setup ? "Pull down to finish" : ""
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
            id: repoDelegate
            width: reposView.width
            height: repoColumn.height

            readonly property bool showGroup: index === 0 || shown.get(index - 1).group !== model.group

            Column {
                id: repoColumn
                width: parent.width
                height: implicitHeight

                Item {
                    width: parent.width
                    visible: repoDelegate.showGroup
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

                ListItem {
                    id: repoItem
                    width: parent.width
                    contentHeight: Theme.itemSizeLarge
                    onClicked: {
                        reposView.currentIndex = -1
                        page.openRow(index)
                    }

                    Rectangle {
                        id: dot
                        anchors.left: parent.left
                        anchors.leftMargin: Theme.horizontalPageMargin
                        anchors.verticalCenter: parent.verticalCenter
                        width: Theme.paddingMedium
                        height: width
                        radius: width / 2
                        color: model.kind === "publisher" ? "#64b5f6" : page.dotColor(model.url, model.color)
                        opacity: model.on ? 1 : 0.35
                    }

                    Column {
                        anchors.left: dot.right
                        anchors.leftMargin: Theme.paddingLarge
                        anchors.right: chevron.left
                        anchors.rightMargin: Theme.paddingMedium
                        anchors.verticalCenter: parent.verticalCenter

                        Label {
                            width: parent.width
                            truncationMode: TruncationMode.Fade
                            color: repoItem.highlighted ? Theme.highlightColor : (model.on ? Theme.primaryColor : Theme.secondaryColor)
                            text: model.title
                        }
                        Label {
                            width: parent.width
                            visible: text !== ""
                            truncationMode: TruncationMode.Fade
                            font.pixelSize: Theme.fontSizeSmall
                            color: repoItem.highlighted ? Theme.secondaryHighlightColor : (model.problem ? Theme.highlightColor : Theme.secondaryColor)
                            text: model.line
                        }
                    }

                    Image {
                        id: chevron
                        anchors.right: parent.right
                        anchors.rightMargin: Theme.horizontalPageMargin
                        anchors.verticalCenter: parent.verticalCenter
                        source: "image://theme/icon-m-right?" + (repoItem.highlighted ? Theme.highlightColor : Theme.primaryColor)
                    }
                }
            }
        }

        ViewPlaceholder {
            enabled: repos.count === 0
            text: "No repositories"
        }

        VerticalScrollDecorator {}
    }
}
