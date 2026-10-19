#include "ftuna.h"

#include "libftuna.h"

#include <PackageKit/Daemon>
#include <PackageKit/Transaction>

#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QtConcurrent/QtConcurrentRun>

#include <unistd.h>

Ftuna::Ftuna(QObject *parent)
    : QObject(parent), m_busy(false), m_refreshing(false), m_installing(false), m_refreshAgain(false), m_onboarded(false), m_haveSystemRepos(false), m_ratingsBaseline(0), m_taskProgress(0) {
    m_timer.setInterval(200);
    connect(&m_timer, &QTimer::timeout, this, &Ftuna::pollProgress);
    m_ratingsTimer.setInterval(200);
    connect(&m_ratingsTimer, &QTimer::timeout, this, &Ftuna::pollRatings);
    m_onboarded = settings().value(QStringLiteral("onboarded")).toBool();
}

int Ftuna::progress() const {
    if (m_installing) {
        return m_taskProgress;
    }
    return FtunaProgress();
}

QString packageKitStatus(PackageKit::Transaction::Status status) {
    switch (status) {
    case PackageKit::Transaction::StatusWait:
    case PackageKit::Transaction::StatusWaitingForLock:
        return QStringLiteral("Waiting");
    case PackageKit::Transaction::StatusWaitingForAuth:
        return QStringLiteral("Waiting for authentication");
    case PackageKit::Transaction::StatusSetup:
        return QStringLiteral("Preparing");
    case PackageKit::Transaction::StatusDepResolve:
    case PackageKit::Transaction::StatusQuery:
        return QStringLiteral("Resolving dependencies");
    case PackageKit::Transaction::StatusDownload:
    case PackageKit::Transaction::StatusDownloadRepository:
    case PackageKit::Transaction::StatusDownloadPackagelist:
    case PackageKit::Transaction::StatusDownloadFilelist:
        return QStringLiteral("Downloading");
    case PackageKit::Transaction::StatusInstall:
    case PackageKit::Transaction::StatusUpdate:
    case PackageKit::Transaction::StatusCommit:
        return QStringLiteral("Installing");
    case PackageKit::Transaction::StatusRemove:
        return QStringLiteral("Removing");
    case PackageKit::Transaction::StatusSigCheck:
        return QStringLiteral("Checking signatures");
    case PackageKit::Transaction::StatusFinished:
        return QStringLiteral("Finishing");
    default:
        return QStringLiteral("Working");
    }
}

QString packageLabel(const QString &packageID) {
    const QStringList parts = packageID.split(QLatin1Char(';'));
    if (parts.isEmpty() || parts.at(0).isEmpty()) {
        return packageID.trimmed();
    }
    QString version = parts.size() > 1 ? parts.at(1) : QString();
    const int colon = version.indexOf(QLatin1Char(':'));
    if (colon >= 0) {
        version = version.mid(colon + 1);
    }
    if (version.isEmpty()) {
        return parts.at(0);
    }
    return parts.at(0) + QLatin1Char(' ') + version;
}

QString packageVerb(PackageKit::Transaction::Info info) {
    switch (info) {
    case PackageKit::Transaction::InfoDownloading:
        return QStringLiteral("Downloading");
    case PackageKit::Transaction::InfoInstalling:
    case PackageKit::Transaction::InfoUpdating:
    case PackageKit::Transaction::InfoReinstalling:
        return QStringLiteral("Installing");
    case PackageKit::Transaction::InfoRemoving:
        return QStringLiteral("Removing");
    case PackageKit::Transaction::InfoDowngrading:
        return QStringLiteral("Downgrading");
    case PackageKit::Transaction::InfoPreparing:
    case PackageKit::Transaction::InfoCleanup:
    case PackageKit::Transaction::InfoObsoleting:
        return QStringLiteral("Preparing");
    default:
        return QString();
    }
}

QString packageVerb(PackageKit::Transaction::Status status) {
    switch (status) {
    case PackageKit::Transaction::StatusDownload:
    case PackageKit::Transaction::StatusDownloadRepository:
    case PackageKit::Transaction::StatusDownloadPackagelist:
    case PackageKit::Transaction::StatusDownloadFilelist:
        return QStringLiteral("Downloading");
    case PackageKit::Transaction::StatusInstall:
    case PackageKit::Transaction::StatusUpdate:
    case PackageKit::Transaction::StatusCommit:
        return QStringLiteral("Installing");
    case PackageKit::Transaction::StatusRemove:
        return QStringLiteral("Removing");
    default:
        return QString();
    }
}

void Ftuna::pollProgress() {
    emit progressChanged();
    if (m_installing) {
        return;
    }
    char *raw = FtunaStatus();
    QString next;
    if (raw) {
        next = QString::fromUtf8(raw);
        FtunaFree(raw);
    }
    if (next == m_activity) {
        return;
    }
    m_activity = next;
    emit activityChanged();
}

QVariantMap Ftuna::settings() {
    char *raw = FtunaSettings();
    if (!raw) {
        return QVariantMap();
    }
    const QByteArray bytes(raw);
    FtunaFree(raw);
    return QJsonDocument::fromJson(bytes).object().toVariantMap();
}

QString Ftuna::saveSettings(const QString &json) {
    const QByteArray bytes = json.toUtf8();
    QByteArray mutableBytes = bytes;
    char *raw = FtunaSaveSettings(mutableBytes.data());
    QString err;
    if (raw) {
        const QByteArray out(raw);
        FtunaFree(raw);
        const QJsonObject obj = QJsonDocument::fromJson(out).object();
        if (obj.isEmpty()) {
            err = QString::fromUtf8(out);
        } else {
            m_onboarded = obj.value(QStringLiteral("onboarded")).toBool();
        }
    }
    emit settingsChanged();
    return err;
}

QString Ftuna::rememberFallbacks(const QString &json) {
    QByteArray bytes = json.toUtf8();
    char *raw = FtunaRememberFallbacks(bytes.data());
    QString err;
    if (raw) {
        err = QString::fromUtf8(raw);
        FtunaFree(raw);
    }
    return err;
}

void Ftuna::refresh() {
    if (m_busy || m_refreshing) {
        // The catalog the user just edited still needs a pass once this task ends.
        m_refreshAgain = true;
        return;
    }
    m_refreshAgain = false;
    m_refreshing = true;
    m_busy = true;
    emit busyChanged();
    m_timer.start();
    QtConcurrent::run([this]() { refreshWork(); });
}

QString ftunaString(char *raw) {
    if (!raw) {
        return QString();
    }
    const QString text = QString::fromUtf8(raw);
    FtunaFree(raw);
    return text;
}

void Ftuna::refreshWork() {
    const int seen = FtunaRatingsGeneration();
    const QString local = ftunaString(FtunaLoadCached());
    if (local != QStringLiteral("0") && local != QStringLiteral("1")) {
        QMetaObject::invokeMethod(this, "refreshDone", Qt::QueuedConnection, Q_ARG(QString, local), Q_ARG(int, seen));
        return;
    }
    const bool shown = local == QStringLiteral("1");
    if (shown) {
        QMetaObject::invokeMethod(this, "catalogReady", Qt::QueuedConnection);
    }
    const QString catalogs = ftunaString(FtunaRefreshCatalogs());
    if (!shown && catalogs.isEmpty()) {
        QMetaObject::invokeMethod(this, "catalogReady", Qt::QueuedConnection);
    }
    const QString ratings = ftunaString(FtunaFetchRatings());
    const QString err = !catalogs.isEmpty() ? catalogs : ratings;
    QMetaObject::invokeMethod(this, "refreshDone", Qt::QueuedConnection, Q_ARG(QString, err), Q_ARG(int, seen));
}

void Ftuna::catalogReady() {
    if (m_installing) {
        emit refreshed();
        return;
    }
    m_busy = false;
    emit busyChanged();
    emit refreshed();
}

void Ftuna::refreshDone(const QString &error, int ratingsSeen) {
    m_timer.stop();
    m_refreshing = false;
    if (!m_installing) {
        m_busy = false;
        // The worker clears its status before this slot runs. Read it once
        // more, or the label stays on the last phase after the timer stops.
        pollProgress();
    }
    if (!error.isEmpty()) {
        m_notice = error;
        emit noticeChanged();
    }
    emit progressChanged();
    emit busyChanged();
    emit refreshed();
    m_ratingsBaseline = ratingsSeen;
    const int now = FtunaRatingsGeneration();
    if (now != m_ratingsBaseline) {
        m_ratingsBaseline = now;
        emit ratingsReady();
    } else {
        m_ratingsTimer.start();
    }
    continueRefresh();
}

void Ftuna::continueRefresh() {
    if (!m_refreshAgain || m_busy || m_refreshing) {
        return;
    }
    refresh();
}

void Ftuna::noteWait() {
    const QString wait = QStringLiteral("Wait for the current task to finish");
    if (m_installResult == wait) {
        return;
    }
    m_installResult = wait;
    emit installResultChanged();
}

void Ftuna::pollRatings() {
    const int now = FtunaRatingsGeneration();
    if (now == m_ratingsBaseline) {
        return;
    }
    m_ratingsTimer.stop();
    m_ratingsBaseline = now;
    emit ratingsReady();
}

QVariantMap Ftuna::state() {
    char *raw = FtunaState();
    if (!raw) {
        return QVariantMap();
    }
    const QByteArray bytes(raw);
    FtunaFree(raw);
    return QJsonDocument::fromJson(bytes).object().toVariantMap();
}

QVariantMap Ftuna::listing() {
    char *raw = FtunaListing();
    if (!raw) {
        return QVariantMap();
    }
    const QByteArray bytes(raw);
    FtunaFree(raw);
    return QJsonDocument::fromJson(bytes).object().toVariantMap();
}

QVariantMap Ftuna::package(const QString &id) {
    QByteArray bytes = id.toUtf8();
    char *raw = FtunaPackage(bytes.data());
    if (!raw) {
        return QVariantMap();
    }
    const QByteArray out(raw);
    FtunaFree(raw);
    const QJsonDocument doc = QJsonDocument::fromJson(out);
    if (!doc.isObject()) {
        return QVariantMap();
    }
    return doc.object().toVariantMap();
}

QVariantList Ftuna::openReposUsers(bool refresh) {
    char *raw = FtunaOpenReposUsers(refresh ? 1 : 0);
    if (!raw) {
        return QVariantList();
    }
    const QByteArray out(raw);
    FtunaFree(raw);
    const QJsonDocument doc = QJsonDocument::fromJson(out);
    if (!doc.isArray()) {
        return QVariantList();
    }
    return doc.array().toVariantList();
}

QVariantList Ftuna::repoPackages(const QString &id) {
    QByteArray bytes = id.toUtf8();
    char *raw = FtunaRepoPackages(bytes.data());
    if (!raw) {
        return QVariantList();
    }
    const QByteArray out(raw);
    FtunaFree(raw);
    const QJsonDocument doc = QJsonDocument::fromJson(out);
    if (!doc.isArray()) {
        return QVariantList();
    }
    return doc.array().toVariantList();
}

QVariantList Ftuna::search(const QString &query) {
    QByteArray bytes = query.toUtf8();
    char *raw = FtunaSearch(bytes.data());
    if (!raw) {
        return QVariantList();
    }
    const QByteArray out(raw);
    FtunaFree(raw);
    return QJsonDocument::fromJson(out).array().toVariantList();
}

QVariantMap Ftuna::report(const QString &id, const QString &source, const QStringList &names) {
    QByteArray idBytes = id.toUtf8();
    QByteArray sourceBytes = source.toUtf8();
    QByteArray nameBytes = QJsonDocument(QJsonArray::fromStringList(names)).toJson(QJsonDocument::Compact);
    char *raw = FtunaReport(idBytes.data(), sourceBytes.data(), nameBytes.data());
    if (!raw) {
        return QVariantMap();
    }
    const QJsonDocument doc = QJsonDocument::fromJson(QByteArray(raw));
    FtunaFree(raw);
    if (!doc.isObject()) {
        return QVariantMap();
    }
    return doc.object().toVariantMap();
}

QStringList Ftuna::dependencies(const QString &id, const QString &source, const QStringList &names) {
    QByteArray idBytes = id.toUtf8();
    QByteArray sourceBytes = source.toUtf8();
    QByteArray nameBytes = QJsonDocument(QJsonArray::fromStringList(names)).toJson(QJsonDocument::Compact);
    char *raw = FtunaDependencies(idBytes.data(), sourceBytes.data(), nameBytes.data());
    QStringList out;
    if (!raw) {
        return out;
    }
    const QJsonArray arr = QJsonDocument::fromJson(QByteArray(raw)).array();
    FtunaFree(raw);
    for (int i = 0; i < arr.size(); ++i) {
        out << arr.at(i).toString();
    }
    return out;
}

QVariantList Ftuna::systemRepos() {
    if (m_busy && m_haveSystemRepos) {
        return m_systemRepos;
    }
    char *raw = FtunaSystemRepos();
    if (!raw) {
        return m_haveSystemRepos ? m_systemRepos : QVariantList();
    }
    const QByteArray bytes(raw);
    FtunaFree(raw);
    const QJsonDocument doc = QJsonDocument::fromJson(bytes);
    if (!doc.isArray()) {
        return m_haveSystemRepos ? m_systemRepos : QVariantList();
    }
    m_systemRepos = doc.array().toVariantList();
    m_haveSystemRepos = true;
    return m_systemRepos;
}

QString Ftuna::wipe() {
    char *raw = FtunaWipe();
    if (!raw) {
        // A refresh worker can recreate cache files until this process is gone.
        _exit(0);
    }
    const QString err = QString::fromUtf8(raw);
    FtunaFree(raw);
    return err;
}

QString Ftuna::setRepoEnabled(const QString &repoURL, bool on) {
    if (m_busy) {
        return QStringLiteral("Wait for the current task to finish");
    }
    QByteArray bytes = repoURL.toUtf8();
    char *raw = FtunaSetRepoEnabled(bytes.data(), on ? 1 : 0);
    QString err;
    if (raw) {
        err = QString::fromUtf8(raw);
        FtunaFree(raw);
    }
    return err;
}

void Ftuna::preview(const QString &repoURL, const QStringList &names) {
    if (m_busy) {
        emit changesFailed(QStringLiteral("Wait for the current task to finish"));
        return;
    }
    m_installing = true;
    m_busy = true;
    m_activity = QStringLiteral("Checking changes");
    m_taskProgress = 0;
    m_previewIds.clear();
    m_previewChanges.clear();
    emit activityChanged();
    emit busyChanged();
    QtConcurrent::run([this, repoURL, names]() { previewWork(repoURL, names); });
}

void Ftuna::installPackages(const QStringList &ids) {
    if (m_busy) {
        noteWait();
        return;
    }
    if (ids.isEmpty()) {
        m_installResult = QStringLiteral("no packages selected");
        emit installResultChanged();
        return;
    }
    m_installing = true;
    m_busy = true;
    m_successText = QStringLiteral("Installed");
    m_activity = QStringLiteral("Installing");
    m_taskProgress = 0;
    m_installResult.clear();
    m_lastLog.clear();
    emit progressChanged();
    emit activityChanged();
    emit installResultChanged();
    emit busyChanged();
    startInstall(ids);
}

void Ftuna::install(const QString &repoURL, const QStringList &names) {
    if (m_busy) {
        noteWait();
        return;
    }
    m_installing = true;
    m_busy = true;
    m_successText = QStringLiteral("Installed");
    m_activity = QStringLiteral("Resolving packages");
    m_taskProgress = 0;
    m_installResult.clear();
    m_lastLog.clear();
    emit progressChanged();
    emit activityChanged();
    emit installResultChanged();
    emit busyChanged();
    QtConcurrent::run([this, repoURL, names]() { installWork(repoURL, names); });
}

void Ftuna::uninstall(const QStringList &names) {
    if (m_busy) {
        noteWait();
        return;
    }
    m_installing = true;
    m_busy = true;
    m_successText = QStringLiteral("Removed");
    m_activity = QStringLiteral("Removing");
    m_taskProgress = 0;
    m_installResult.clear();
    m_lastLog.clear();
    emit progressChanged();
    emit activityChanged();
    emit installResultChanged();
    emit busyChanged();
    QtConcurrent::run([this, names]() { uninstallWork(names); });
}

void Ftuna::uninstallWork(const QStringList &names) {
    QString err;
    QStringList ids;
    QByteArray encoded = QJsonDocument(QJsonArray::fromStringList(names)).toJson(QJsonDocument::Compact);
    char *raw = FtunaInstalledIDs(encoded.data());
    if (raw) {
        const QByteArray out(raw);
        FtunaFree(raw);
        const QJsonDocument doc = QJsonDocument::fromJson(out);
        if (doc.isArray()) {
            const QJsonArray arr = doc.array();
            for (int i = 0; i < arr.size(); ++i) {
                ids << arr.at(i).toString();
            }
        } else {
            err = QString::fromUtf8(out);
        }
    }
    if (!err.isEmpty() || ids.isEmpty()) {
        if (err.isEmpty()) {
            err = QStringLiteral("none of the selected packages are installed");
        }
        QMetaObject::invokeMethod(this, "installDone", Qt::QueuedConnection, Q_ARG(QString, err));
        return;
    }
    QMetaObject::invokeMethod(this, "startRemove", Qt::QueuedConnection, Q_ARG(QStringList, ids));
}

void Ftuna::previewWork(const QString &repoURL, const QStringList &names) {
    QByteArray repo = repoURL.toUtf8();
    QByteArray encoded = QJsonDocument(QJsonArray::fromStringList(names)).toJson(QJsonDocument::Compact);
    char *raw = FtunaPreview(repo.data(), encoded.data());
    QString payload;
    if (raw) {
        payload = QString::fromUtf8(raw);
        FtunaFree(raw);
    }
    QMetaObject::invokeMethod(this, "applyPreview", Qt::QueuedConnection, Q_ARG(QString, payload));
}

void Ftuna::applyPreview(const QString &payload) {
    const QJsonDocument doc = QJsonDocument::fromJson(payload.toUtf8());
    if (!doc.isObject()) {
        finishPreview(payload.isEmpty() ? QStringLiteral("Could not check changes") : payload);
        return;
    }
    const QJsonObject obj = doc.object();
    m_previewIds.clear();
    const QJsonArray ids = obj.value(QStringLiteral("ids")).toArray();
    for (int i = 0; i < ids.size(); ++i) {
        m_previewIds << ids.at(i).toString();
    }
    m_previewChanges.clear();
    const QJsonArray changes = obj.value(QStringLiteral("changes")).toArray();
    for (int i = 0; i < changes.size(); ++i) {
        m_previewChanges.append(changes.at(i).toObject().toVariantMap());
    }
    finishPreview(QString());
}

void Ftuna::finishPreview(const QString &error) {
    m_installing = false;
    m_busy = false;
    m_activity.clear();
    emit activityChanged();
    emit busyChanged();
    if (error.isEmpty()) {
        emit changesReady(m_previewChanges);
        return;
    }
    emit changesFailed(error);
}

void Ftuna::installWork(const QString &repoURL, const QStringList &names) {
    QString err;
    QStringList ids;
    QByteArray repo = repoURL.toUtf8();
    QByteArray encoded = QJsonDocument(QJsonArray::fromStringList(names)).toJson(QJsonDocument::Compact);
    char *raw = FtunaPackageIDs(repo.data(), encoded.data());
    if (raw) {
        const QByteArray out(raw);
        FtunaFree(raw);
        const QJsonDocument doc = QJsonDocument::fromJson(out);
        if (doc.isArray()) {
            const QJsonArray arr = doc.array();
            for (int i = 0; i < arr.size(); ++i) {
                ids << arr.at(i).toString();
            }
        } else {
            err = QString::fromUtf8(out);
        }
    }
    if (!err.isEmpty() || ids.isEmpty()) {
        if (err.isEmpty()) {
            err = QStringLiteral("no packages selected");
        }
        QMetaObject::invokeMethod(this, "installDone", Qt::QueuedConnection, Q_ARG(QString, err));
        return;
    }
    QMetaObject::invokeMethod(this, "startInstall", Qt::QueuedConnection, Q_ARG(QStringList, ids));
}

void Ftuna::startInstall(const QStringList &ids) {
    for (const QString &id : ids) {
        note(packageLabel(id));
    }
    startTransaction(PackageKit::Daemon::installPackages(ids), QStringLiteral("Install failed"));
}

void Ftuna::startRemove(const QStringList &ids) {
    startTransaction(PackageKit::Daemon::removePackages(ids), QStringLiteral("Remove failed"));
}

void Ftuna::startTransaction(PackageKit::Transaction *tx, const QString &failed) {
    m_pkError.clear();
    connect(tx, &PackageKit::Transaction::percentageChanged, this, [this, tx]() {
        const uint pct = tx->percentage();
        if (pct > 100 || m_taskProgress == int(pct)) {
            return;
        }
        m_taskProgress = int(pct);
        emit progressChanged();
    });
    connect(tx, &PackageKit::Transaction::statusChanged, this, [this, tx]() {
        const QString next = packageKitStatus(tx->status());
        if (next == m_activity) {
            return;
        }
        m_activity = next;
        emit activityChanged();
    });
    connect(tx, &PackageKit::Transaction::package, this,
            [this](PackageKit::Transaction::Info info, const QString &packageID, const QString &) {
                const QString verb = packageVerb(info);
                if (!verb.isEmpty()) {
                    note(verb + QLatin1Char(' ') + packageLabel(packageID));
                }
            });
    connect(tx, &PackageKit::Transaction::itemProgress, this,
            [this](const QString &id, PackageKit::Transaction::Status status, uint) {
                const QString verb = packageVerb(status);
                if (!verb.isEmpty() && !id.isEmpty()) {
                    note(verb + QLatin1Char(' ') + packageLabel(id));
                }
            });
    connect(tx, &PackageKit::Transaction::errorCode, this,
            [this](PackageKit::Transaction::Error, const QString &details) {
                m_pkError = details;
                note(details);
            });
    connect(tx, &PackageKit::Transaction::finished, this,
            [this, failed](PackageKit::Transaction::Exit status, uint) {
                const QString err = status == PackageKit::Transaction::ExitSuccess
                    ? QString()
                    : (m_pkError.isEmpty() ? failed : m_pkError);
                installDone(err);
            });
}

void Ftuna::note(const QString &line) {
    const QString text = line.trimmed();
    if (text.isEmpty() || text == m_lastLog) {
        return;
    }
    m_lastLog = text;
    emit logLine(text);
}

void Ftuna::installDone(const QString &error) {
    m_installing = false;
    m_busy = false;
    m_activity.clear();
    m_installResult = error.isEmpty() ? m_successText : error;
    emit activityChanged();
    emit installResultChanged();
    emit progressChanged();
    emit busyChanged();
    continueRefresh();
}
