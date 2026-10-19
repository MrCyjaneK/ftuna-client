#pragma once

#include <QObject>
#include <QStringList>
#include <QTimer>
#include <QVariant>

namespace PackageKit {
class Transaction;
}

class Ftuna : public QObject {
    Q_OBJECT
    Q_PROPERTY(bool busy READ busy NOTIFY busyChanged)
    Q_PROPERTY(int progress READ progress NOTIFY progressChanged)
    Q_PROPERTY(QString activity READ activity NOTIFY activityChanged)
    Q_PROPERTY(bool onboarded READ onboarded NOTIFY settingsChanged)
    Q_PROPERTY(QString notice READ notice NOTIFY noticeChanged)
    Q_PROPERTY(QString installResult READ installResult NOTIFY installResultChanged)
    Q_PROPERTY(QStringList resolvedIds READ resolvedIds NOTIFY changesReady)

public:
    explicit Ftuna(QObject *parent = nullptr);
    bool busy() const { return m_busy; }
    int progress() const;
    QString activity() const { return m_activity; }
    bool onboarded() const { return m_onboarded; }
    QString notice() const { return m_notice; }
    QString installResult() const { return m_installResult; }
    QStringList resolvedIds() const { return m_previewIds; }

    Q_INVOKABLE QVariantMap settings();
    Q_INVOKABLE QString saveSettings(const QString &json);
    Q_INVOKABLE QString rememberFallbacks(const QString &json);
    Q_INVOKABLE void refresh();
    Q_INVOKABLE QVariantMap state();
    Q_INVOKABLE QVariantMap listing();
    Q_INVOKABLE QVariantMap package(const QString &id);
    Q_INVOKABLE QVariantList search(const QString &query);
    Q_INVOKABLE QVariantList repoPackages(const QString &id);
    Q_INVOKABLE QVariantList openReposUsers(bool refresh);
    Q_INVOKABLE QVariantMap report(const QString &id, const QString &source, const QStringList &names);
    Q_INVOKABLE QStringList dependencies(const QString &id, const QString &source, const QStringList &names);
    Q_INVOKABLE void install(const QString &repoURL, const QStringList &names);
    Q_INVOKABLE void installPackages(const QStringList &ids);
    Q_INVOKABLE void preview(const QString &repoURL, const QStringList &names);
    Q_INVOKABLE void uninstall(const QStringList &names);
    Q_INVOKABLE QVariantList systemRepos();
    Q_INVOKABLE QString setRepoEnabled(const QString &repoURL, bool on);
    Q_INVOKABLE QString wipe();

signals:
    void busyChanged();
    void progressChanged();
    void activityChanged();
    void settingsChanged();
    void noticeChanged();
    void installResultChanged();
    void logLine(const QString &line);
    void changesReady(const QVariantList &changes);
    void changesFailed(const QString &error);
    void refreshed();
    void ratingsReady();

private slots:
    void refreshDone(const QString &error, int ratingsSeen);
    void catalogReady();
    void pollProgress();
    void pollRatings();
    void startInstall(const QStringList &ids);
    void startRemove(const QStringList &ids);
    void applyPreview(const QString &payload);
    void finishPreview(const QString &error);
    void installDone(const QString &error);

private:
    void refreshWork();
    void installWork(const QString &repoURL, const QStringList &names);
    void previewWork(const QString &repoURL, const QStringList &names);
    void uninstallWork(const QStringList &names);
    void startTransaction(PackageKit::Transaction *tx, const QString &failed);
    void note(const QString &line);
    void noteWait();
    void continueRefresh();

    bool m_busy;
    bool m_refreshing;
    bool m_installing;
    bool m_refreshAgain;
    bool m_onboarded;
    QString m_notice;
    QString m_activity;
    QString m_installResult;
    QString m_successText;
    QString m_pkError;
    QString m_lastLog;
    QStringList m_previewIds;
    QVariantList m_previewChanges;
    QVariantList m_systemRepos;
    bool m_haveSystemRepos;
    QTimer m_timer;
    QTimer m_ratingsTimer;
    int m_ratingsBaseline;
    int m_taskProgress;
};
