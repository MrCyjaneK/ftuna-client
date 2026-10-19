.pragma library

var safe = "#5dbe7e"
var caution = "#e0a040"
var risk = "#e15b64"
var openrepos = "#ff9800"
var chum = "#2e7d32"

function markColor(name, unknown) {
    if (name === "green")
        return safe
    if (name === "yellow")
        return caution
    if (name === "red")
        return risk
    return unknown
}

// segments splits a finding count into bar pieces.
// Order is red, yellow, gray, green. A missing count is skipped.
// Each piece is {color, x, w} with x and w in 0..1.
function segments(counts) {
    var order = ["red", "yellow", "gray", "green"]
    var parts = []
    var total = 0
    var i
    for (i = 0; i < order.length; i++) {
        var n = counts && counts[order[i]] ? counts[order[i]] : 0
        if (n > 0) {
            parts.push({color: order[i], n: n})
            total += n
        }
    }
    if (total === 0)
        return [{color: "gray", x: 0, w: 1}]
    var x = 0
    var out = []
    for (i = 0; i < parts.length; i++) {
        var w = parts[i].n / total
        if (i === parts.length - 1)
            w = 1 - x
        out.push({color: parts[i].color, x: x, w: w})
        x += parts[i].n / total
    }
    return out
}

function repoColor(name, fallback) {
    var value = (name || "").toLowerCase()
    if (value.indexOf("openrepos") >= 0 || value.indexOf("or/") === 0)
        return openrepos
    if (value.indexOf("chum") >= 0 || value.indexOf("oss") >= 0)
        return chum
    return fallback
}

function picture(raw) {
    if (!raw)
        return ""
    if (raw.indexOf("://") >= 0)
        return raw
    return "file://" + raw
}

function iconSource(raw, fallback) {
    var shown = picture(raw)
    return shown !== "" ? shown : fallback
}

function selectedSpecs(model) {
    var specs = []
    for (var i = 0; i < model.count; i++) {
        var row = model.get(i)
        if (!row.checked)
            continue
        if (row.rpmVersion || row.rpmRelease)
            specs.push(row.name + "\t" + (row.rpmVersion || "") + "\t" + (row.rpmRelease || ""))
        else
            specs.push(row.name)
    }
    return specs
}

function openPackage(stack, backend, id, pageUrl) {
    if (!backend || !id)
        return
    var full = backend.package(id)
    if (!full || !full.id)
        return
    stack.push(pageUrl, { backend: backend, pkg: full })
}

function trim(value) {
    return (value || "").replace(/^\s+|\s+$/g, "")
}

function byId(packages) {
    var map = {}
    var list = packages || []
    for (var i = 0; i < list.length; i++)
        map[list[i].id] = list[i]
    return map
}

function idsOf(ids, map) {
    var out = []
    var list = ids || []
    for (var i = 0; i < list.length; i++) {
        if (map[list[i]])
            out.push(map[list[i]])
    }
    return out
}
