import QtQuick 2.6
import Sailfish.Silica 1.0
import "../components"
import "../ui.js" as Ui

Page {
    id: page
    property var backend
    property var pkg

    property int sourceIndex: 0
    property int repoTick: 0
    property string repoError: ""
    property var phoneRepos: []

    function reloadPhoneRepos() {
        phoneRepos = backend.systemRepos() || []
    }

    function descriptionText(raw) {
        var text = raw || ""
        var at = text.lastIndexOf("<img src=\"")
        if (at < 0 || text.indexOf("icons/theme/ai-score/svgs/icons/AI-score-", at) < 0)
            return text
        return text.substring(0, at).replace(/\s+$/, "")
    }

    function badgeSource(raw) {
        var text = raw || ""
        var names = ["will-not-tell", "unknown", "H", "A", "B", "C", "V"]
        for (var i = 0; i < names.length; i++) {
            if (text.indexOf("AI-score-" + names[i] + ".svg") >= 0)
                return Qt.resolvedUrl("../images/AI-score-" + names[i] + ".png")
        }
        return Qt.resolvedUrl("../images/AI-score-unknown.png")
    }

    function repoKey(url) {
        return (url || "").replace(/\/+$/, "")
    }

    function chumChannel(url) {
        var key = repoKey(url) + "/"
        var testing = "https://repo.sailfishos.org/obs/sailfishos:/chum:/testing/"
        var stable = "https://repo.sailfishos.org/obs/sailfishos:/chum/"
        if (key.indexOf(testing) === 0)
            return "testing"
        if (key.indexOf(stable) === 0)
            return "stable"
        return ""
    }

    function chumOwned(repo) {
        var alias = repo.alias || ""
        if (alias !== "sailfishos-chum" && alias !== "sailfishos-chum-testing")
            return false
        return chumChannel(repo.url) !== ""
    }

    function repoOn(url) {
        var _tick = repoTick
        var want = repoKey(url)
        var wantChannel = chumChannel(want)
        for (var i = 0; i < phoneRepos.length; i++) {
            var repo = phoneRepos[i]
            var got = repoKey(repo.url)
            if (got === want)
                return _tick >= 0
            if (wantChannel !== "" && chumChannel(got) === wantChannel)
                return _tick >= 0
            if (wantChannel !== "" && chumOwned(repo))
                return _tick >= 0
        }
        return false
    }

    onRepoTickChanged: reloadPhoneRepos()
    readonly property var sources: (pkg.sources && pkg.sources.length) ? pkg.sources : [{
        title: pkg.source || pkg.repo || "Repository",
        enabled: pkg.source_enabled === true,
        repo_url: pkg.repo_url || "",
        updated: pkg.updated,
        summary: pkg.summary,
        description: pkg.description,
        icon_path: pkg.icon_path,
        screenshot_paths: pkg.screenshot_paths || [],
        status: pkg.status,
        rpms: pkg.rpms || []
    }]
    readonly property var active: sources[Math.min(sourceIndex, sources.length - 1)]

    property bool sampleReport: false
    property var reportDeps: []
    property var chosen: []
    readonly property var shots: (active && active.screenshot_paths && active.screenshot_paths.length) ? active.screenshot_paths : (pkg.screenshot_paths || [])
    property string outcome: ""
    readonly property bool canRemove: {
        var text = (outcome || "").toLowerCase()
        return text === "installed" || (text.indexOf("already") >= 0 && text.indexOf("installed") >= 0)
    }
    property int selected: 0

    ListModel { id: rpmModel }
    property var lineCounts: ({})
    property var reportPackages: []

    // sourceAt reads the array by index. The active binding can still be the previous source
    // when a click handler runs, which made the rating lag one tap behind.
    function sourceAt(index) {
        var list = sources
        if (!list || list.length === 0)
            return null
        var i = index
        if (i < 0)
            i = 0
        if (i >= list.length)
            i = list.length - 1
        return list[i]
    }

    property string reportSource: ""

    function rebuildReport() {
        var rep = backend ? backend.report(pkg.id, reportSource, Ui.selectedSpecs(rpmModel)) : {}
        if (!rep)
            rep = {}
        page.sampleReport = rep.sample === true
        page.lineCounts = rep.line || {}
        page.reportDeps = rep.dependencies || []
        page.reportPackages = rep.packages || []
    }

    function loadPackages() {
        var src = sourceAt(sourceIndex)
        reportSource = src && src.id ? src.id : ""
        rpmModel.clear()
        var list = (src && src.rpms) ? src.rpms : (pkg.rpms || [])
        for (var i = 0; i < list.length; i++) {
            var rpm = list[i]
            var version = rpm.version || ""
            if (rpm.release)
                version = version ? version + "-" + rpm.release : rpm.release
            rpmModel.append({
                name: rpm.name,
                rpmVersion: rpm.version || "",
                rpmRelease: rpm.release || "",
                version: version,
                checked: rpm.install !== false,
                lineRed: (rpm.line && rpm.line.red) || 0,
                lineYellow: (rpm.line && rpm.line.yellow) || 0,
                lineGray: (rpm.line && rpm.line.gray) || 0,
                lineGreen: (rpm.line && rpm.line.green) || 0,
                requires: (rpm.requires || []).join(", ")
            })
        }
        recount()
    }

    function recount() {
        var n = 0
        var rows = []
        for (var i = 0; i < rpmModel.count; i++) {
            var row = rpmModel.get(i)
            if (!row.checked)
                continue
            n++
            rows.push({
                label: row.version ? row.name + "  " + row.version : row.name,
                marks: {
                    red: row.lineRed || 0,
                    yellow: row.lineYellow || 0,
                    gray: row.lineGray || 0,
                    green: row.lineGreen || 0
                }
            })
        }
        selected = n
        chosen = rows
        rebuildReport()
    }

    function install() {
        var specs = Ui.selectedSpecs(rpmModel)
        if (specs.length === 0)
            return
        var src = sourceAt(sourceIndex)
        var url = src && src.repo_url ? src.repo_url : (pkg.repo_url || "")
        if (!url) {
            var st = backend.state()
            url = st.repo_url
        }
        var lines = []
        for (var i = 0; i < rpmModel.count; i++) {
            var row = rpmModel.get(i)
            if (!row.checked)
                continue
            lines.push(row.version ? row.name + "  " + row.version : row.name)
        }
        var summary = (src && src.summary) ? src.summary : (pkg.summary || pkg.id)
        pageStack.push(Qt.resolvedUrl("ChangePage.qml"), {
            backend: backend,
            action: "Install",
            packages: [{
                id: pkg.id,
                summary: summary,
                icon_path: (src && src.icon_path) ? src.icon_path : (pkg.icon_path || ""),
                repo: (src && src.title) ? src.title : (pkg.source || pkg.repo || ""),
                color: "",
                installed: "",
                version: "",
                repo_url: url || "",
                specs: specs,
                lines: lines
            }]
        })
    }

    function selectedNames() {
        var names = []
        for (var i = 0; i < rpmModel.count; i++) {
            if (rpmModel.get(i).checked)
                names.push(rpmModel.get(i).name)
        }
        return names
    }

    function uninstall() {
        var names = selectedNames()
        if (names.length === 0)
            return
        remorse.execute("Uninstalling", function() {
            backend.uninstall(names)
        })
    }

    function indexOfSource(list, id) {
        if (!list || !id)
            return 0
        for (var i = 0; i < list.length; i++) {
            if (list[i].id === id)
                return i
        }
        return 0
    }

    function preferredIndex() {
        var want = pkg && pkg.preferred_source ? pkg.preferred_source : ""
        return indexOfSource(sources, want)
    }

    function reloadAnalysis() {
        if (!backend || !pkg || !pkg.id)
            return
        var previous = pkg.preferred_source || ""
        var keep = sourceIndex
        var currentId = ""
        if (sources && keep >= 0 && keep < sources.length)
            currentId = sources[keep].id || ""
        var full = backend.package(pkg.id)
        if (!full || !full.id)
            return
        var followedPreferred = previous === "" ? keep === 0 : currentId === previous
        pkg = full
        sourceIndex = followedPreferred ? indexOfSource(full.sources, full.preferred_source) : keep
        loadPackages()
    }

    function showSource(index) {
        sourceIndex = index
        loadPackages()
    }

    Connections {
        target: backend
        onInstallResultChanged: page.outcome = backend.installResult
        onRatingsReady: page.reloadAnalysis()
        onRefreshed: page.reloadAnalysis()
    }

    Component.onCompleted: {
        sourceIndex = preferredIndex()
        reloadPhoneRepos()
        loadPackages()
    }

    onStatusChanged: {
        if (status === PageStatus.Active)
            reloadPhoneRepos()
    }

    SilicaFlickable {
        anchors.fill: parent
        contentHeight: column.height + Theme.paddingLarge

        PullDownMenu {
            MenuItem {
                text: "Install"
                enabled: page.selected > 0
                onClicked: page.install()
            }
            MenuItem {
                text: "Uninstall"
                visible: page.canRemove
                enabled: page.selected > 0 && !backend.busy
                onClicked: page.uninstall()
            }
        }

        Column {
            id: column
            width: parent.width
            spacing: Theme.paddingLarge

            PackageHeader {
                title: (active && active.summary) ? active.summary : (pkg.summary || pkg.id)
                description: (active && active.summary) || pkg.summary ? pkg.id : ""
                iconSource: Ui.picture(active && active.icon_path ? active.icon_path : pkg.icon_path)
            }

            BackgroundItem {
                id: descriptionItem
                property bool expanded: false
                property string body: (active && active.description) ? active.description : (pkg.description || "")
                property string prose: page.descriptionText(body)
                property string badge: page.badgeSource(body)
                width: parent.width
                height: descriptionColumn.height
                clip: true
                propagateComposedEvents: true
                highlighted: down && !descriptionPress.running
                onPressedChanged: if (pressed) descriptionPress.restart()
                onClicked: if (bodyLabel.truncated || expanded) expanded = !expanded

                Behavior on height { NumberAnimation { duration: 200; easing.type: Easing.InOutQuad } }

                Timer {
                    id: descriptionPress
                    interval: 50
                }

                Column {
                    id: descriptionColumn
                    x: Theme.horizontalPageMargin
                    width: parent.width - 2 * x
                    spacing: Theme.paddingSmall

                    Item {
                        visible: descriptionItem.prose !== ""
                        width: parent.width
                        height: visible ? bodyLabel.height : 0

                        Label {
                            id: bodyLabel
                            width: parent.width
                            wrapMode: Text.Wrap
                            font.pixelSize: Theme.fontSizeSmall
                            color: descriptionItem.highlighted ? Theme.highlightColor : Theme.primaryColor
                            maximumLineCount: descriptionItem.expanded ? 0 : 7
                            text: descriptionItem.prose
                        }

                        OpacityRampEffect {
                            sourceItem: bodyLabel
                            enabled: !descriptionItem.expanded && bodyLabel.truncated
                            direction: OpacityRamp.TopToBottom
                        }
                    }

                    Image {
                        anchors.right: parent.right
                        visible: descriptionItem.expanded || bodyLabel.truncated
                        source: "image://theme/icon-lock-more"
                    }

                    Image {
                        source: descriptionItem.badge
                        width: Theme.itemSizeLarge
                        height: Theme.iconSizeLarge
                        fillMode: Image.PreserveAspectFit
                    }
                }
            }

            Row {
                width: parent.width
                visible: shots.length > 0

                Repeater {
                    model: Math.min(shots.length, 3)
                    Image {
                        width: column.width / 3
                        height: width
                        fillMode: Image.PreserveAspectCrop
                        source: Ui.picture(shots[index])
                        MouseArea {
                            anchors.fill: parent
                            onClicked: pageStack.animatorPush(Qt.resolvedUrl("ScreenshotPage.qml"), {
                                paths: shots,
                                index: index
                            })
                        }
                    }
                }
            }

            Column {
                width: parent.width

                SectionHeader { text: "Package" }
                DetailItem {
                    label: "Category"
                    value: pkg.category || "—"
                }
                DetailItem {
                    label: "Updated"
                    value: {
                        var updated = active && active.updated ? active.updated : pkg.updated
                        return updated ? updated.substring(0, 10) : "—"
                    }
                }
            }

            Column {
                width: parent.width

                SectionHeader { text: "Source" }
                Repeater {
                    model: page.sources
                    BackgroundItem {
                        id: sourceRow
                        width: parent.width
                        height: Theme.itemSizeSmall
                        highlighted: index === page.sourceIndex
                        onClicked: page.showSource(index)

                        readonly property color sourceColor: Ui.repoColor(modelData.title || modelData.id || "",
                                                                          highlighted ? Theme.highlightColor : Theme.primaryColor)

                        Rectangle {
                            id: sourceDot
                            x: Theme.horizontalPageMargin
                            anchors.verticalCenter: parent.verticalCenter
                            width: Theme.paddingMedium
                            height: width
                            radius: width / 2
                            color: sourceRow.sourceColor
                        }
                        Switch {
                            id: sourceToggle
                            anchors.right: parent.right
                            anchors.rightMargin: Theme.horizontalPageMargin
                            anchors.verticalCenter: parent.verticalCenter
                            visible: (modelData.repo_url || "") !== ""
                            automaticCheck: false
                            checked: page.repoOn(modelData.repo_url)
                            onClicked: {
                                var turnOn = !page.repoOn(modelData.repo_url)
                                page.repoError = backend.setRepoEnabled(modelData.repo_url, turnOn)
                                page.repoTick++
                            }
                        }
                        Label {
                            anchors.left: sourceDot.right
                            anchors.leftMargin: Theme.paddingMedium
                            anchors.right: sourceToggle.visible ? sourceToggle.left : parent.right
                            anchors.rightMargin: Theme.paddingMedium
                            anchors.verticalCenter: parent.verticalCenter
                            truncationMode: TruncationMode.Fade
                            color: sourceRow.sourceColor
                            text: {
                                var title = modelData.title || modelData.id || "Repository"
                                var version = modelData.version || ""
                                return version ? title + "  " + version : title
                            }
                        }
                    }
                }
                Label {
                    x: Theme.horizontalPageMargin
                    width: parent.width - 2 * x
                    visible: page.repoError !== ""
                    wrapMode: Text.Wrap
                    color: "#e15b64"
                    text: page.repoError
                }
            }

            BackgroundItem {
                id: packageTile
                width: parent.width
                height: (page.chosen.length > 0 ? packageLabel.height : emptyPackages.height) + Theme.paddingLarge
                visible: rpmModel.count > 0
                onClicked: pageStack.push(Qt.resolvedUrl("PackageSelectPage.qml"), {
                    backend: backend,
                    packageId: pkg.id,
                    rpmModel: rpmModel,
                    host: page
                })

                Column {
                    id: packageLabel
                    x: Theme.horizontalPageMargin
                    y: Theme.paddingMedium
                    width: parent.width - x - packageChevron.width - Theme.horizontalPageMargin - Theme.paddingMedium
                    visible: page.chosen.length > 0
                    spacing: Theme.paddingSmall

                    Repeater {
                        model: page.chosen
                        Item {
                            width: packageLabel.width
                            height: nameText.height + Theme.paddingSmall

                            Label {
                                id: nameText
                                width: parent.width
                                truncationMode: TruncationMode.Fade
                                color: packageTile.highlighted ? Theme.highlightColor : Theme.primaryColor
                                text: modelData.label
                            }
                            ColorLine {
                                anchors.left: parent.left
                                anchors.right: parent.right
                                anchors.top: nameText.bottom
                                anchors.topMargin: Theme.paddingSmall / 2
                                height: Math.max(3, Theme.paddingSmall / 2)
                                counts: modelData.marks
                            }
                        }
                    }
                }
                Label {
                    id: emptyPackages
                    x: Theme.horizontalPageMargin
                    y: Theme.paddingMedium
                    visible: page.chosen.length === 0
                    color: packageTile.highlighted ? Theme.highlightColor : Theme.primaryColor
                    text: "No package selected"
                }
                Image {
                    id: packageChevron
                    anchors.right: parent.right
                    anchors.rightMargin: Theme.horizontalPageMargin
                    anchors.verticalCenter: parent.verticalCenter
                    source: "image://theme/icon-m-right"
                }
            }

            BackgroundItem {
                id: reportTile
                width: parent.width
                height: Theme.itemSizeLarge
                onClicked: pageStack.push(Qt.resolvedUrl("ReportPage.qml"), { host: page })

                Column {
                    anchors.left: parent.left
                    anchors.leftMargin: Theme.horizontalPageMargin
                    anchors.right: reportChevron.left
                    anchors.rightMargin: Theme.paddingMedium
                    anchors.verticalCenter: parent.verticalCenter

                    Label {
                        id: findingsLabel
                        width: parent.width
                        truncationMode: TruncationMode.Fade
                        font.pixelSize: Theme.fontSizeLarge
                        color: reportTile.highlighted ? Theme.highlightColor : Theme.primaryColor
                        text: "Findings"
                    }
                    ColorLine {
                        width: parent.width
                        height: Math.max(3, Theme.paddingSmall / 2)
                        counts: page.lineCounts
                    }
                }
                Image {
                    id: reportChevron
                    anchors.right: parent.right
                    anchors.rightMargin: Theme.horizontalPageMargin
                    anchors.verticalCenter: parent.verticalCenter
                    source: "image://theme/icon-m-right"
                }
            }

            Label {
                x: Theme.horizontalPageMargin
                width: parent.width - 2 * x
                readonly property string statusLine: outcome !== "" ? outcome : (backend.busy ? (backend.activity !== "" ? backend.activity : "Working") : "")
                visible: statusLine !== ""
                wrapMode: Text.Wrap
                color: outcome === "Installed" || outcome === "Removed" ? "#5dbe7e"
                       : (backend.busy || outcome.indexOf("Wait for") === 0) ? Theme.highlightColor
                       : "#e15b64"
                text: statusLine
            }
        }
    }

    RemorsePopup { id: remorse }
}
