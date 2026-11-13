import Foundation

@MainActor
enum AppDelegate {
    static var relaunchOnExit = false
}

private struct TestFailure: Error, CustomStringConvertible {
    let description: String
}

private func expect(_ condition: Bool, _ message: String) throws {
    if !condition { throw TestFailure(description: message) }
}

@MainActor
private final class Fixture {
    let directory: URL
    let defaults: UserDefaults
    let suite = "proxycat-recovery-tests-\(UUID().uuidString)"
    let helper: HelperClient
    var now = Date(timeIntervalSince1970: 1_800_000_000)

    lazy var viewModel = StatusViewModel(
        helper: helper, defaults: defaults,
        wifiSSIDProvider: { [weak self] in
            guard let self else { return nil }
            return self.has("campus-arrived") ? "Campus" : "Home"
        }, currentVersion: "0.12.4", currentReleaseNotes: "## 修复\n- 离线更新日志",
        dateProvider: { [weak self] in self?.now ?? Date() }
    )

    init(coreRunning: Bool = true, automaticUpdateChecks: Bool = false) throws {
        directory = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        defaults = UserDefaults(suiteName: suite)!
        defaults.set(["Campus"], forKey: "protectedWiFiSSIDs")
        defaults.set("disconnect", forKey: "campusNetworkAction")
        if !automaticUpdateChecks {
            defaults.set(["checkedAt": now, "succeeded": true], forKey: "updateCheckHistory.0.12.4")
        }
        let executable = directory.appendingPathComponent("proxyctl")
        try #"""
        #!/bin/sh
        fixture_dir=${0%/*}
        log="$fixture_dir/commands.log"
        echo "$*" >> "$log"
        case "$1 $2" in
            'connections prepare')
                if [ -f "$fixture_dir/fail-connection-prepare" ]; then exit 1; fi
                exit 0
                ;;
            'connections --json')
                if [ -f "$fixture_dir/hold-connections" ]; then
                    touch "$fixture_dir/connections-held"
                    while [ -f "$fixture_dir/hold-connections" ]; do sleep 0.02; done
                fi
                if [ -f "$fixture_dir/fail-connections" ]; then echo 'engine unavailable' >&2; exit 1; fi
                cat "$fixture_dir/connections.json"
                ;;
            'website-check '*)
                if [ -f "$fixture_dir/hold-website" ]; then
                    touch "$fixture_dir/website-held"
                    while [ -f "$fixture_dir/hold-website" ]; do sleep 0.02; done
                fi
                if [ -f "$fixture_dir/website-result.json" ]; then cat "$fixture_dir/website-result.json"; else echo 'invalid website' >&2; exit 2; fi
                ;;
            'core status')
                active=false
                if [ -f "$fixture_dir/core-active" ]; then active=true; fi
                printf '{"running":%s,"pid":42}\n' "$active"
                ;;
            'diagnose --json')
                if [ -f "$fixture_dir/fail-diagnose" ]; then echo 'diagnosis unavailable' >&2; exit 8; fi
                if [ -f "$fixture_dir/hold-diagnose" ]; then
                    touch "$fixture_dir/diagnose-held"
                    while [ -f "$fixture_dir/hold-diagnose" ]; do sleep 0.02; done
                fi
                if [ -f "$fixture_dir/diagnose-result.json" ]; then cat "$fixture_dir/diagnose-result.json"; exit 0; fi
                echo '{"app":"ProxyCat","milestone":"test","checks":[{"name":"listener","status":"pass","message":"ready"}]}'
                ;;
            'core start')
                if [ -f "$fixture_dir/hold-start" ]; then
                    touch "$fixture_dir/start-held"
                    while [ -f "$fixture_dir/hold-start" ]; do sleep 0.02; done
                fi
                if [ -f "$fixture_dir/fail-start" ]; then
                    echo 'saved config failed to start' >&2
                    exit 7
                fi
                touch "$fixture_dir/core-active"
                if [ -f "$fixture_dir/diagnose-after-start.json" ]; then cp "$fixture_dir/diagnose-after-start.json" "$fixture_dir/diagnose-result.json"; fi
                ;;
            'core stop') rm -f "$fixture_dir/core-active" "$fixture_dir/tun-active" ;;
            'tun on')
                if [ -f "$fixture_dir/fail-tun" ]; then echo '已取消管理员授权' >&2; exit 8; fi
                if [ -f "$fixture_dir/hold-tun" ]; then
                    printf '%s\n' '{"stage":"start","message":"建立 TUN 网络接口"}'
                    touch "$fixture_dir/tun-held"
                    while [ -f "$fixture_dir/hold-tun" ]; do sleep 0.02; done
                fi
                touch "$fixture_dir/core-active"
                if [ ! -f "$fixture_dir/unready-tun" ]; then touch "$fixture_dir/tun-active"; fi
                ;;
            'tun off') rm -f "$fixture_dir/tun-active" ;;
            'system-proxy status')
                active=false
                if [ -f "$fixture_dir/proxy-active" ]; then active=true; fi
                printf '{"httpEnabled":false,"httpHost":"","httpPort":0,"httpsEnabled":%s,"httpsHost":"","httpsPort":0,"socksEnabled":false,"socksHost":"","socksPort":0}\n' "$active"
                ;;
            'system-proxy on') touch "$fixture_dir/proxy-active" ;;
            'system-proxy off') rm -f "$fixture_dir/proxy-active" ;;
            'mode status')
                if [ -f "$fixture_dir/fail-mode-status" ]; then echo 'controller unavailable' >&2; exit 8; fi
                if [ -f "$fixture_dir/hold-mode" ]; then
                    touch "$fixture_dir/mode-held"
                    while [ -f "$fixture_dir/hold-mode" ]; do sleep 0.02; done
                fi
                mode=rule
                if [ -f "$fixture_dir/mode" ]; then mode=$(cat "$fixture_dir/mode"); fi
                tun=false
                if [ -f "$fixture_dir/tun-active" ]; then tun=true; fi
                printf '{"mode":"%s","tunEnabled":%s,"tunDevice":"utun9"}\n' "$mode" "$tun"
                ;;
            'mode set')
                if [ -f "$fixture_dir/fail-mode" ]; then
                    echo 'global selection unavailable' >&2
                    exit 8
                fi
                printf '%s' "$3" > "$fixture_dir/mode"
                ;;
            'groups select') ;;
            'groups list')
                if [ -f "$fixture_dir/hold-groups" ]; then
                    touch "$fixture_dir/groups-held"
                    while [ -f "$fixture_dir/hold-groups" ]; do sleep 0.02; done
                fi
                if [ -f "$fixture_dir/nodegroups.json" ]; then cat "$fixture_dir/nodegroups.json"; exit 0; fi
                if [ -f "$fixture_dir/enter-campus" ]; then
                    touch "$fixture_dir/campus-arrived"
                    echo '{}'
                    exit 0
                fi
                if [ -f "$fixture_dir/transient-groups" ]; then
                    rm "$fixture_dir/transient-groups"
                    touch "$fixture_dir/empty-once"
                    echo 'controller temporarily unavailable' >&2
                    exit 8
                fi
                if [ -f "$fixture_dir/empty-once" ]; then
                    rm "$fixture_dir/empty-once"
                    echo '{}'
                    exit 0
                fi
                if [ -f "$fixture_dir/fail-groups" ]; then
                    echo 'controller temporarily unavailable' >&2
                    exit 8
                fi
                if [ -f "$fixture_dir/empty-groups" ] || [ ! -f "$fixture_dir/core-active" ]; then
                    echo '{}'
                    exit 0
                fi
                echo 'groups ready' >> "$log"
                if [ -f "$fixture_dir/multi-groups" ]; then
                    echo '{"GLOBAL":{"name":"GLOBAL","type":"Selector","now":"Auto","all":["DIRECT","Auto"]},"Auto":{"name":"Auto","type":"URLTest","now":"Node A","all":["Node A","Node B"]},"Proxy":{"name":"Proxy","type":"Selector","now":"Auto","all":["Auto","DIRECT"]}}'
                    exit 0
                fi
                echo '{"Auto":{"name":"Auto","type":"Selector","now":"Node A","all":["Node A","Node B"]}}'
                ;;
            'profile list')
                if [ -f "$fixture_dir/fail-profiles" ]; then exit 8; fi
                if [ -f "$fixture_dir/profiles.json" ]; then cat "$fixture_dir/profiles.json"; exit 0; fi
                echo '[{"id":"saved","name":"Saved","url":"https://example.invalid/subscription","createdAt":"","updatedAt":"","active":true}]'
                ;;
            'maintenance status')
                if [ -f "$fixture_dir/maintenance.json" ]; then
                    cat "$fixture_dir/maintenance.json"
                elif [ -f "$fixture_dir/stale-maintenance" ]; then
                    echo '{"subscriptionLastSuccess":"2026-01-01T00:00:00Z"}'
                elif [ -f "$fixture_dir/subscription-updated" ]; then
                    echo '{"subscriptionLastSuccess":"2027-01-15T08:00:00Z"}'
                else
                    echo '{"subscriptionLastAttempt":"2027-01-15T08:00:00Z"}'
                fi
                ;;
            'preferences list')
                if [ -f "$fixture_dir/preferences.json" ]; then cat "$fixture_dir/preferences.json"; else echo '{"preferences":[]}'; fi
                ;;
            'preferences set'|'preferences remove')
                if [ -f "$fixture_dir/hold-preference" ]; then
                    touch "$fixture_dir/preference-held"
                    while [ -f "$fixture_dir/hold-preference" ]; do sleep 0.02; done
                fi
                if [ -f "$fixture_dir/fail-preference" ]; then echo 'application failed; restored previous configuration' >&2; exit 7; fi
                cat "$fixture_dir/preference-result.json"
                ;;
            'subscription update')
                if [ -f "$fixture_dir/hold-subscription" ]; then
                    touch "$fixture_dir/subscription-held"
                    while [ -f "$fixture_dir/hold-subscription" ]; do sleep 0.02; done
                fi
                if [ -f "$fixture_dir/subscription-result-maintenance.json" ]; then
                    cp "$fixture_dir/subscription-result-maintenance.json" "$fixture_dir/maintenance.json"
                fi
                if [ -f "$fixture_dir/fail-subscription" ]; then echo 'subscription HTTP 503; previous configuration retained' >&2; exit 7; fi
                touch "$fixture_dir/subscription-updated"
                rm -f "$fixture_dir/stale-maintenance"
                echo '{"stage":"done","message":"订阅更新完成"}'
                ;;
            'profile traffic')
                if [ -f "$fixture_dir/fail-traffic" ]; then echo 'temporary metadata failure' >&2; exit 8; fi
                cat "$fixture_dir/traffic-$3.json"
                ;;
            'proxy-delay Auto') echo '{"delay":42}' ;;
            'proxy-delay Proxy') echo '{"delay":43}' ;;
            'proxy-delay GLOBAL') echo '{"delay":44}' ;;
            'proxy-delay DIRECT') echo '{"delay":45}' ;;
            'self-update --check-only')
                if [ -f "$fixture_dir/hold-check" ]; then
                    while [ -f "$fixture_dir/hold-check" ]; do sleep 0.02; done
                fi
                if [ -f "$fixture_dir/fail-check" ]; then
                    echo '{"stage":"error","message":"更新检查暂时不可用，请稍后重试"}'
                    exit 1
                fi
                if [ -f "$fixture_dir/no-update" ]; then
                    echo '{"stage":"done","message":"已经是最新版","releaseVersion":"0.12.4"}'
                    exit 0
                fi
                printf '%s\n' '{"stage":"done","message":"发现新版本 0.13.0","newVersion":"0.13.0","releaseVersion":"0.13.0","releaseNotes":"## 新功能\n- **更新日志**：查看说明后安装\n## 修复\n- 连接测试","releaseURL":"https://github.com/Cass-ette/ProxyCat/releases/tag/v0.13.0","publishedAt":"2026-10-26T08:00:00Z"}'
                ;;
            'self-update --json')
                if [ "$3" != '--expected-version' ] || [ "$4" != '0.13.0' ]; then exit 9; fi
                if [ -f "$fixture_dir/new-release" ]; then
                    echo '{"stage":"error","message":"可用版本已变化，请重新检查更新","recheck":true}'
                    exit 1
                fi
                if [ -f "$fixture_dir/network-update" ]; then
                    echo '{"stage":"error","message":"安装包下载失败：无法连接更新下载服务。请检查网络；部分网络需要开启代理或 VPN 后重试。","details":"Get https://github.com/app.dmg: dial tcp 20.205.243.166:443: i/o timeout"}'
                    exit 1
                fi
                if [ -f "$fixture_dir/truncated-update" ]; then
                    echo '{"stage":"downloading","message":"下载中","progress":20}'
                    exit 7
                fi
                echo '{"stage":"done","message":"更新完成 0.13.0","newVersion":"0.13.0","progress":100}'
                ;;
            *) echo 'unexpected command' >&2; exit 9 ;;
        esac
        """#.write(to: executable, atomically: true, encoding: .utf8)
        try FileManager.default.setAttributes([.posixPermissions: 0o755], ofItemAtPath: executable.path)
        helper = HelperClient(proxyctlPath: executable.path)
        if coreRunning { try set("core-active") }
    }

    deinit {
        defaults.removePersistentDomain(forName: suite)
        try? FileManager.default.removeItem(at: directory)
    }

    func has(_ flag: String) -> Bool {
        FileManager.default.fileExists(atPath: directory.appendingPathComponent(flag).path)
    }

    func set(_ flag: String) throws {
        try Data().write(to: directory.appendingPathComponent(flag))
    }

    func remove(_ flag: String) throws {
        try FileManager.default.removeItem(at: directory.appendingPathComponent(flag))
    }

    var commands: [String] {
        let text = (try? String(contentsOf: directory.appendingPathComponent("commands.log"), encoding: .utf8)) ?? ""
        return text.split(separator: "\n").map(String.init)
    }

    var updateCheckCount: Int {
        commands.filter { $0 == "self-update --check-only --json" }.count
    }
}

@main
struct ConnectionRecoveryTests {
    @MainActor
    static func main() async throws {
        try await startsSavedCoreBeforeEnablingProxy()
        try await retriesTransientControllerFailures()
        try await unavailableControllerNeverEnablesProxy()
        try await refreshKeepsNodesUntilCoreStops()
        try await savedProfilesSurviveHelperFailure()
        try await failedCoreStartLeavesProxyOff()
        try await campusArrivalCancelsRecovery()
        try await enablingDuringRefreshPublishesRealProxyStatus()
        try await repeatedEnableDoesNotStartTwice()
        try await selectionAndDelayFollowMode()
        try await failedModeChangeKeepsSelection()
        try await reviewBeforeInstallingUpdate()
        try await interruptedUpdateIsRetryable()
        try await downloadNetworkFailureIsRetryable()
        try await updateFailuresRemainReviewable()
        try await automaticUpdatesRequireAnActiveConnection()
        try await automaticUpdatesPreserveReviewAndSurviveRelaunch()
        try await backgroundSubscriptionUpdatesWaitForConnection()
        try await backgroundSubscriptionRespectsDailyBoundaryAndRelaunch()
        try await backgroundSubscriptionRetriesFailuresAfterThirtyMinutes()
        try await backgroundSubscriptionDoesNotBlockOrOverlap()
        try await websitePreferencesReloadFromStorage()
        try websitePreferenceEvidenceRequiresTheSavedRule()
        try await websitePreferenceAppliesAndVerifiesTheRequest()
        try await websitePreferenceDoesNotCertifyUnknownRoutesOrFailures()
        try await websitePreferenceDefersTUNAndNonRuleMode()
        try await websitePreferenceRemovalRechecksSubscriptionRoute()
        try await websitePreferenceVerificationRejectsChangedContext()
        try await websitePreferenceSerializesLifecycleControls()
        try await automaticUpdateSuccessIsThrottled()
        try await automaticUpdateFailuresAreSilentAndRetryable()
        try await automaticUpdateChecksDoNotOverlap()
        try await manualCheckCanJoinABackgroundFailure()
        try await manualUpdatesBypassAutomaticThrottling()
        try legacyUpdateEventsStillDecode()
        try releaseNotesKeepSectionsAndContent()
        try await menuBarTracksProxyState()
        try await tunFollowsConfirmedStateAndCampusPolicy()
        try await tunFailureDoesNotClaimConnection()
        try await completedTUNSwitchDoesNotWaitForNodeRefresh()
        try await unavailableCoreDiagnosisSurvivesPolling()
        try await failedDiagnosisKeepsLastReport()
        try await pendingDiagnosisKeepsLastReport()
        try await diagnosisCanBeCleared()
        try await networkChangesInvalidatePreviousDiagnosis()
        try await lateDiagnosisCannotEraseSwitchFailure()
        try await observedChangesInvalidateDiagnosis()
        try await unreadableControllerInvalidatesDiagnosis()
        try diagnosticModelsDoNotOverstateOldOrIncompleteResults()
        try await diagnosticRecoveryAlwaysVerifiesTheListener()
        try await diagnosticRecoveryPreservesFailures()
        try await diagnosticRecoveryGuardsTUNAndCampusPolicy()
        try await diagnosticRecoveryDoesNotOverlapSwitches()
        try await diagnosticProxyEnableIsExplicitAndRechecked()
        try await diagnosticResultsCannotEraseANewerError()
        try await tunHelperRemovalRequiresInactiveTunnel()
        try websiteCopyDoesNotInventFacts()
        try await websiteRecoveryMatchesEvidence()
        try await websiteGlobalCanBeRestored()
        try await websiteResultsExpireOnContextChanges()
        try await automaticNodeChangesExplainOnlyObservedFacts()
        try latencyStatesAreDistinct()
        try connectionRecordsKeepRouteEvidenceAndApplicationGroups()
        try connectionRecordsKeepBoundedRecentHistory()
        try await connectionObservationPreservesRecordsOnFailure()
        try await closingRecordsDiscardsPendingObservation()
        try await unavailableProcessLookupKeepsConnectionRecords()
        try trafficPresentationKeepsUnknownAndExhaustedStatesHonest()
        try subscriptionExpirationIsVisibleAndKeepsCachedDatesHonest()
        try await trafficRefreshIsIndependentAndThrottled()
        try await failedTrafficRefreshPreservesOnlyCachedUsage()
        try await tunProgressArrivesBeforeTheCommandFinishes()
        try await menuBarTrafficWarningsPreserveConnectionColors()
        try await nodeNamesAreFormattedOnlyForDisplay()
        print("Passed 74 connection, diagnostic, website, node, mode, TUN, menu bar, update and subscription traffic regression tests")
    }

    static func trafficPresentationKeepsUnknownAndExhaustedStatesHonest() throws {
        let legacy = try JSONDecoder().decode(Profile.self, from: Data(#"{"id":"a","name":"Plan","url":"https://example.invalid","createdAt":"","updatedAt":"","active":true}"#.utf8))
        try expect(legacy.traffic == nil && TrafficPresentation(profile: legacy).label == "流量未知", "Existing subscriptions must decode without invented usage")
        var plan = legacy
        plan.traffic = SubscriptionTraffic(upload: 1024 * 1024 * 1024, download: 213 * 1024 * 1024 * 1024, total: 300 * 1024 * 1024 * 1024)
        plan.trafficCheckedAt = "2026-11-10T13:00:00Z"
        let now = ISO8601DateFormatter().date(from: "2026-11-10T13:05:00Z")!
        let display = TrafficPresentation(profile: plan, now: now)
        try expect(display.label == "剩余 86 / 300 GB" && abs((display.fraction ?? -1) - 86.0 / 300.0) < 0.0001, "Battery shows remaining and total upload plus download allowance")
        let cached = TrafficPresentation(profile: plan, refreshError: "failed", now: now)
        try expect(cached.label == "上次剩余 86 / 300 GB", "A failed refresh must label the balance as cached")
        plan.traffic = SubscriptionTraffic(upload: 10, download: 120, total: 100)
        try expect(plan.traffic?.remaining == 0 && plan.traffic?.remainingFraction == 0, "Overage must clamp to empty, never wrap or become negative")
        plan.traffic = SubscriptionTraffic(upload: Int64.max, download: Int64.max, total: Int64.max)
        try expect(plan.traffic?.remaining == 0, "Malformed large provider counters must not overflow")
        plan.traffic = SubscriptionTraffic(upload: 10, download: 20, total: 0)
        try expect(TrafficPresentation(profile: plan, now: now).fraction == nil, "Zero total does not prove unlimited or full allowance")
        plan.traffic = SubscriptionTraffic(upload: 0, download: 0, total: 100, expire: 1)
        try expect(TrafficPresentation(profile: plan, now: now).isExpired, "Expiry is separate from byte allowance")
    }

    static func subscriptionExpirationIsVisibleAndKeepsCachedDatesHonest() throws {
        let now = ISO8601DateFormatter().date(from: "2026-11-11T10:00:00Z")!
        var plan = Profile(id: "plan", name: "Plan", url: "", createdAt: "", updatedAt: "", active: true)
        plan.trafficCheckedAt = ISO8601DateFormatter().string(from: now)
        func display(expiringIn interval: TimeInterval) -> TrafficPresentation {
            plan.traffic = SubscriptionTraffic(upload: 10, download: 20, total: 100,
                                               expire: Int64(now.addingTimeInterval(interval).timeIntervalSince1970))
            return TrafficPresentation(profile: plan, now: now)
        }
        let day: TimeInterval = 24 * 60 * 60
        let upcoming = display(expiringIn: 3 * day + 60)
        try expect(upcoming.expirationSummary?.contains("剩4天") == true && upcoming.expirationSummary?.contains("到期") == true,
                   "The closed allowance control must expose the expiry date and round partial days up")
        try expect(upcoming.expirationNeedsAttention && !upcoming.expirationIsUrgent,
                   "Expiry within seven days needs a subtle warning separate from low traffic")
        let distant = display(expiringIn: 10 * day)
        try expect(!distant.expirationNeedsAttention && !distant.expirationIsUrgent,
                   "A distant expiration must keep the normal secondary text color")
        let tomorrow = display(expiringIn: day)
        try expect(tomorrow.expirationSummary?.contains("剩1天") == true && tomorrow.expirationIsUrgent,
                   "Exactly one day must not be described as less than a day")
        let today = display(expiringIn: 60)
        try expect(today.expirationSummary?.contains("不足1天") == true && today.expirationIsUrgent,
                   "A subscription expiring in minutes must not appear to have a whole day left")
        let expired = display(expiringIn: -60)
        try expect(expired.expirationSummary?.contains("已到期") == true && expired.expirationIsUrgent,
                   "Passed expiry must be explicit instead of showing a negative countdown")
        var cached = display(expiringIn: 60)
        cached.refreshError = "failed"
        try expect(cached.expirationSummary?.hasPrefix("上次：") == true && !cached.expirationNeedsAttention && !cached.expirationIsUrgent,
                   "A failed refresh must identify the previous expiry date without a current warning")
        var old = display(expiringIn: 60)
        old.now = now.addingTimeInterval(31 * 60)
        try expect(old.expirationSummary?.hasPrefix("上次：") == true && !old.expirationIsUrgent,
                   "Expired metadata must not claim that a possibly renewed subscription is currently expired")
        let crossYear = display(expiringIn: 400 * day)
        try expect(crossYear.expirationSummary?.contains("2027/") == true,
                   "An expiration outside the current year must include its year")
        plan.traffic = SubscriptionTraffic(upload: 0, download: 0, total: 100)
        try expect(TrafficPresentation(profile: plan, now: now).expirationSummary == "到期时间未提供",
                   "Missing provider expiry must remain unknown, never inferred from allowance")
        plan.traffic = nil
        try expect(TrafficPresentation(profile: plan, now: now).expirationSummary == nil && TrafficPresentation(profile: nil).expirationSummary == nil,
                   "No subscription metadata must not invent an expiration line")
    }

    @MainActor
    private static func writeTraffic(_ fixture: Fixture) throws {
        let timestamp = ISO8601DateFormatter().string(from: fixture.now)
        let response = "{\"profileID\":\"saved\",\"traffic\":{\"upload\":10,\"download\":20,\"total\":100},\"checkedAt\":\"\(timestamp)\"}"
        try response.write(to: fixture.directory.appendingPathComponent("traffic-saved.json"), atomically: true, encoding: .utf8)
    }

    @MainActor
    static func trafficRefreshIsIndependentAndThrottled() async throws {
        let fixture = try Fixture(coreRunning: false)
        try writeTraffic(fixture)
        await fixture.viewModel.loadProfiles()
        fixture.viewModel.lastError = "existing connection error"
        await fixture.viewModel.refreshSubscriptionTraffic()
        try expect(fixture.viewModel.activeProfile?.traffic?.remaining == 70 && !fixture.viewModel.isRefreshingTraffic, "Metadata must publish without starting the engine")
        try expect(fixture.viewModel.lastError == "existing connection error", "Traffic refresh must not erase network failures")
        await fixture.viewModel.refreshSubscriptionTraffic()
        try expect(fixture.commands.filter { $0 == "profile traffic saved --json" }.count == 1, "Opening the menu repeatedly must reuse fresh account data")
        try expect(fixture.commands.allSatisfy { $0.hasPrefix("profile ") }, "Quota reads cannot start, stop, reload or enable networking")
    }

    @MainActor
    static func failedTrafficRefreshPreservesOnlyCachedUsage() async throws {
        let fixture = try Fixture()
        try writeTraffic(fixture)
        await fixture.viewModel.loadProfiles()
        await fixture.viewModel.refreshSubscriptionTraffic()
        try fixture.set("fail-traffic")
        await fixture.viewModel.refreshSubscriptionTraffic(force: true)
        try expect(fixture.viewModel.activeProfile?.traffic?.remaining == 70 && fixture.viewModel.trafficRefreshError != nil,
                   "A metadata outage must preserve and mark the last known balance")
        try expect(!fixture.viewModel.isRefreshingTraffic && fixture.viewModel.lastError == nil,
                   "Quota failure must be retryable without replacing connection errors")
    }

    @MainActor
    static func tunProgressArrivesBeforeTheCommandFinishes() async throws {
        let fixture = try Fixture()
        try fixture.set("hold-tun")
        let operation = Task { await fixture.viewModel.setTUNEnabled(true) }
        for _ in 0..<200 {
            if fixture.viewModel.tunProgress?.stage == "start" { break }
            try await Task.sleep(nanoseconds: 10_000_000)
        }
        let receivedDuringSwitch = fixture.viewModel.isChangingTUN && fixture.viewModel.tunProgress?.stage == "start"
        let hasStartTime = fixture.viewModel.tunChangeStartedAt != nil
        try fixture.remove("hold-tun")
        await operation.value
        try expect(receivedDuringSwitch && hasStartTime, "Actual helper progress must reach the UI while the lifecycle command is still running")
        try expect(fixture.viewModel.tunProgress == nil && fixture.viewModel.tunChangeStartedAt == nil && !fixture.viewModel.isChangingTUN,
                   "A completed switch must clear its timer and progress without leaving an animation running")
    }

    @MainActor
    static func menuBarTrafficWarningsPreserveConnectionColors() async throws {
        let fixture = try Fixture()
        fixture.viewModel.coreRunning = true
        fixture.viewModel.systemProxyEnabled = true
        var plan = Profile(id: "saved", name: "Plan", url: "", createdAt: "", updatedAt: "", active: true)
        plan.trafficCheckedAt = ISO8601DateFormatter().string(from: Date())
        fixture.viewModel.activeProfileID = plan.id
        var indicators: [MenuBarIndicator] = []
        let observer = MenuBarStatusObserver(viewModel: fixture.viewModel) { indicators.append($0) }
        for (download, warning) in [(Int64(85), TrafficWarning.low), (95, .critical), (100, .exhausted)] {
            plan.traffic = SubscriptionTraffic(upload: 0, download: download, total: 100)
            fixture.viewModel.profiles = [plan]
            try await Task.sleep(nanoseconds: 20_000_000)
            try expect(indicators.last?.warning == warning && indicators.last?.state == .connected,
                       "Quota badges must update independently without replacing the connected cat color")
            try expect(indicators.last?.image()?.isTemplate == false, "The badge color must survive template rendering")
        }
        plan.trafficCheckedAt = "2020-01-01T00:00:00Z"
        fixture.viewModel.profiles = [plan]
        try await Task.sleep(nanoseconds: 20_000_000)
        try expect(indicators.last?.warning == nil && indicators.last?.description.contains("上次剩余") == true,
                   "Stale provider data must not claim a current low-quota warning")
        fixture.viewModel.activeProfileID = "other"
        try await Task.sleep(nanoseconds: 20_000_000)
        try expect(indicators.last?.warning == nil && indicators.last?.trafficSummary == nil, "Switching profiles must remove the previous subscription’s warning")
        withExtendedLifetime(observer) {}
    }

    @MainActor
    static func nodeNamesAreFormattedOnlyForDisplay() async throws {
        let raw = "\u{1F1F9}\u{1F1FC} 台湾-A"
        try expect(ProxyDisplay.name(raw) == "\u{1F1E8}\u{1F1F3} 台湾-A", "Taiwan flags must be normalized before macOS hides the original glyph")
        try expect(ProxyDisplay.name("台湾-A") == "\u{1F1E8}\u{1F1F3} 台湾-A", "Names without a visible flag still need the expected display flag")
        try expect(ProxyDisplay.name("\u{1F1F8}\u{1F1EC} 新加坡-A") == "\u{1F1F8}\u{1F1EC} 新加坡-A", "Other node flags remain intact")
        let fixture = try Fixture()
        await fixture.viewModel.selectProxy(group: "Auto", proxy: raw)
        try expect(fixture.commands.contains("groups select Auto \(raw)"), "Selection must send the exact original node identity to the controller")
    }

    static func connection(_ id: String, app: String = "Browser", route: String = "proxy", rule: String = "Match", payload: String = "") -> ConnectionRecord {
        ConnectionRecord(id: id, app: app, host: "example.com", destinationPort: "443", network: "tcp", inbound: "Tun",
                         route: route, rule: rule, rulePayload: payload, chains: route == "direct" ? ["DIRECT"] : ["Singapore A", "Proxy"],
                         node: route == "direct" ? "DIRECT" : "Singapore A", startedAt: "")
    }

    static func connectionRecordsKeepRouteEvidenceAndApplicationGroups() throws {
        var log = ConnectionLog()
        let proxy = connection("a")
        let direct = connection("b", app: "curl", route: "direct", rule: "GeoIP", payload: "CN")
        let unknownApp = connection("c", app: "")
        log.observe(ConnectionSnapshot(observedAt: "", connections: [proxy, direct, unknownApp]), at: Date())
        try expect(log.groups(matching: "").count == 3 && unknownApp.appLabel == "未识别应用", "unknown apps must not be guessed")
        try expect(log.groups(matching: "").last?.name == "未识别应用", "known application groups should appear first")
        try expect(!proxy.reason.contains("海外") && proxy.reason.contains("默认"), "MATCH does not prove an overseas website")
        try expect(direct.reason.contains("国内 IP") && direct.routeLabel == "直连", "direct route and CN evidence remain distinct")
        try expect(log.groups(matching: "Singapore").flatMap(\.entries).count == 2, "node search must preserve grouping")
        try expect(log.groups(matching: "CURL").first?.entries.count == 1, "application search must ignore case")
        try expect(log.groups(matching: "missing").isEmpty, "search should not invent unseen traffic")
    }

    static func connectionRecordsKeepBoundedRecentHistory() throws {
        var log = ConnectionLog()
        let now = Date()
        log.observe(ConnectionSnapshot(observedAt: "", connections: [connection("a"), connection("b")]), at: now)
        log.observe(ConnectionSnapshot(observedAt: "", connections: [connection("a"), connection("a")]), at: now.addingTimeInterval(2))
        try expect(log.entries.count == 2 && log.entries.filter(\.isActive).count == 1, "deduplicate IDs and mark vanished sockets as ended")
        try expect(log.entries.first?.firstSeen == now && log.entries.first?.lastSeen == now.addingTimeInterval(2), "refresh must retain first observation")
        log.clearEnded()
        try expect(log.entries.count == 1 && log.entries[0].isActive, "clear must retain active records")
        log.observe(ConnectionSnapshot(observedAt: "", connections: (0..<400).map { connection("new-\($0)") }), at: now.addingTimeInterval(4))
        try expect(log.entries.count == ConnectionLog.capacity && log.entries.allSatisfy(\.isActive), "memory bound must favor current connections")
        log.observe(ConnectionSnapshot(observedAt: "", connections: []), at: now.addingTimeInterval(6))
        try expect(log.entries.allSatisfy { !$0.isActive }, "an empty successful snapshot must end observed connections")
    }

    @MainActor
    static func connectionObservationPreservesRecordsOnFailure() async throws {
        let fixture = try Fixture()
        let snapshot = ConnectionSnapshot(observedAt: "2026-11-10T00:00:00Z", connections: [connection("a")])
        try JSONEncoder().encode(snapshot).write(to: fixture.directory.appendingPathComponent("connections.json"))
        await fixture.viewModel.refreshConnections()
        try expect(fixture.viewModel.connectionLog.entries.count == 1, "helper snapshot must decode and populate records")
        try fixture.set("fail-connections")
        await fixture.viewModel.refreshConnections()
        try expect(fixture.viewModel.connectionLog.entries[0].isActive && fixture.viewModel.connectionLogError != nil, "failed polling must retain stale evidence, not end connections")
        try fixture.remove("fail-connections")
        try JSONEncoder().encode(ConnectionSnapshot(observedAt: "", connections: [])).write(to: fixture.directory.appendingPathComponent("connections.json"))
        await fixture.viewModel.refreshConnections()
        try expect(fixture.viewModel.connectionLogError == nil && !fixture.viewModel.connectionLog.entries[0].isActive, "successful polling must recover and update state")
        try expect(fixture.commands.allSatisfy { $0 == "connections --json" }, "records must never change mode, TUN or core lifecycle")
    }

    @MainActor
    static func closingRecordsDiscardsPendingObservation() async throws {
        let fixture = try Fixture()
        try JSONEncoder().encode(ConnectionSnapshot(observedAt: "", connections: [connection("a")])).write(to: fixture.directory.appendingPathComponent("connections.json"))
        try fixture.set("hold-connections")
        fixture.viewModel.startObservingConnections()
        fixture.viewModel.startObservingConnections()
        for _ in 0..<100 {
            if fixture.has("connections-held") { break }
            try await Task.sleep(nanoseconds: 20_000_000)
        }
        try expect(fixture.has("connections-held"), "polling did not start")
        fixture.viewModel.stopObservingConnections()
        try fixture.remove("hold-connections")
        for _ in 0..<100 {
            if !fixture.viewModel.isLoadingConnections { break }
            try await Task.sleep(nanoseconds: 20_000_000)
        }
        try expect(fixture.viewModel.connectionLog.entries.isEmpty && fixture.commands.count == 2, "closed window must ignore pending output and duplicate starts")
        await fixture.viewModel.refreshConnections()
        try expect(fixture.viewModel.connectionLog.entries.count == 1, "polling can resume after cancellation")
    }

    @MainActor
    static func unavailableProcessLookupKeepsConnectionRecords() async throws {
        let fixture = try Fixture()
        try JSONEncoder().encode(ConnectionSnapshot(observedAt: "", connections: [connection("a", app: "")])).write(to: fixture.directory.appendingPathComponent("connections.json"))
        try fixture.set("fail-connection-prepare")
        fixture.viewModel.startObservingConnections()
        for _ in 0..<100 {
            if !fixture.viewModel.connectionLog.entries.isEmpty { break }
            try await Task.sleep(nanoseconds: 20_000_000)
        }
        fixture.viewModel.stopObservingConnections()
        try expect(fixture.viewModel.connectionLog.entries.count == 1 && fixture.viewModel.connectionLookupNotice != nil, "process lookup failure must explain the limitation and still show connections")
        try expect(fixture.commands == ["connections prepare", "connections --json"], "opening records may only enable attribution and read connections")
    }

    static func websiteFixture(_ route: String = "proxy", outcome: String = "timeout", mode: String = "rule") throws -> Data {
        try JSONSerialization.data(withJSONObject: ["host":"example.com", "scheme":"https", "outcome":outcome, "statusCode":outcome == "denied" ? 403 : 0, "elapsedMS":100, "route":route, "rule":"Match", "rulePayload":"", "chains": route == "proxy" ? ["Node A","Auto"] : ["DIRECT"], "node":route == "proxy" ? "Node A" : "DIRECT", "group": route == "proxy" ? "Auto" : "", "mode":mode, "observedAt":"2026-11-09T12:00:00Z"])
    }

    static func websiteCopyDoesNotInventFacts() throws {
        let result = try JSONDecoder().decode(WebsiteCheckResult.self, from: websiteFixture())
        try expect(result.reason.contains("默认") && !result.summary.contains("海外"), "MATCH is not geographic evidence")
        let denied = try JSONDecoder().decode(WebsiteCheckResult.self, from: websiteFixture(outcome: "denied"))
        try expect(!denied.connectionFailed && denied.outcomeLabel.contains("访问受限"), "HTTP 403 does not imply a dead node")
    }

    @MainActor
    static func websiteRecoveryMatchesEvidence() async throws {
        let fixture = try Fixture()
        await fixture.viewModel.refreshStatus()
        try websiteFixture().write(to: fixture.directory.appendingPathComponent("website-result.json"))
        await fixture.viewModel.checkWebsite("https://example.com")
        try expect(fixture.viewModel.websiteRecoveryGroups.map(\.name) == ["Auto"], "Only selectors on the observed chain can be recovered")
        await fixture.viewModel.changeNodeAndRecheck(group: "Unrelated", proxy: "Node B")
        try expect(!fixture.commands.contains("groups select Unrelated Node B"), "Cannot mutate unrelated groups")
        await fixture.viewModel.changeNodeAndRecheck(group: "Auto", proxy: "Node B")
        try expect(fixture.commands.contains("groups select Auto Node B") && fixture.commands.filter { $0.hasPrefix("website-check ") }.count == 2, "Changing an observed selector must rerun the website probe")
    }

    @MainActor
    static func websiteGlobalCanBeRestored() async throws {
        let fixture = try Fixture()
        await fixture.viewModel.refreshStatus()
        try websiteFixture("direct").write(to: fixture.directory.appendingPathComponent("website-result.json"))
        await fixture.viewModel.checkWebsite("https://example.com")
        await fixture.viewModel.checkWebsiteUsingGlobal()
        try expect(fixture.viewModel.currentMode == "global" && fixture.viewModel.websitePreviousMode == "rule", "A global recovery keeps a restore affordance")
        await fixture.viewModel.restoreWebsiteMode()
        try expect(fixture.viewModel.currentMode == "rule" && fixture.viewModel.websitePreviousMode == nil, "Restore original mode explicitly")
        try fixture.set("fail-mode")
        await fixture.viewModel.checkWebsite("https://example.com")
        await fixture.viewModel.checkWebsiteUsingGlobal()
        try expect(fixture.viewModel.currentMode == "rule" && fixture.viewModel.websitePreviousMode == nil, "Failed switch cannot claim global mode")
    }

    @MainActor
    static func websiteResultsExpireOnContextChanges() async throws {
        let fixture = try Fixture()
        await fixture.viewModel.refreshStatus()
        try websiteFixture().write(to: fixture.directory.appendingPathComponent("website-result.json"))
        await fixture.viewModel.checkWebsite("https://example.com")
        fixture.viewModel.activeProfileID = "different-profile"
        try expect(!fixture.viewModel.websiteResultIsCurrent && fixture.viewModel.websiteRecoveryGroups.isEmpty, "Old evidence must not change another profile")
        await fixture.viewModel.changeNodeAndRecheck(group: "Auto", proxy: "Node B")
        try expect(!fixture.commands.contains("groups select Auto Node B"), "Reject stale recovery")
    }

    @MainActor
    static func automaticNodeChangesExplainOnlyObservedFacts() async throws {
        let fixture = try Fixture()
        func groups(_ node: String) throws {
            let data = Data("{\"Auto\":{\"name\":\"Auto\",\"type\":\"URLTest\",\"now\":\"\(node)\",\"all\":[\"Node A\",\"Node B\"]}}".utf8)
            try data.write(to: fixture.directory.appendingPathComponent("nodegroups.json"))
        }
        try groups("Node A")
        await fixture.viewModel.refreshStatus()
        try expect(fixture.viewModel.nodeChangeNotice == nil, "Initial snapshot is not a switch")
        try groups("Node B")
        await fixture.viewModel.refreshStatus()
        try expect(fixture.viewModel.nodeChangeNotice?.message.contains("Node A 切换到 Node B") == true && fixture.viewModel.nodeChangeNotice?.message.contains("连不上") == false, "Change evidence cannot invent a failure cause")
        fixture.viewModel.nodeChangeNotice = nil
        await fixture.viewModel.refreshStatus()
        try expect(fixture.viewModel.nodeChangeNotice == nil, "Unchanged refresh must not repeat a dismissed notice")
        try groups("\u{1F1F9}\u{1F1FC} 台湾-A")
        await fixture.viewModel.refreshStatus()
        try expect(fixture.viewModel.nodeChangeNotice?.message.contains("\u{1F1E8}\u{1F1F3} 台湾-A") == true,
                   "Automatic node changes must use the same visible flag as the node list")
        try groups("Node A")
        await fixture.viewModel.selectProxy(group: "Auto", proxy: "Node A")
        try expect(fixture.viewModel.nodeChangeNotice == nil, "Manual selection is not an automatic change")
    }

    static func latencyStatesAreDistinct() throws {
        try expect(ProxyDelayResult(delay: 0, error: nil).quality == "未测试", "Zero is not a fast measurement")
        try expect(ProxyDelayResult(delay: 0, error: "timeout").quality == "不可用", "Failure differs from untested")
        try expect([42,150,800].map { ProxyDelayResult(delay: $0, error: nil).quality } == ["快","一般","慢"], "Latency categories stay readable")
        try expect(ProxyDelayResult(delay: 800, error: nil).detail.contains("800 ms"), "Keep precise latency accessible")
    }

    @MainActor
    static func startsSavedCoreBeforeEnablingProxy() async throws {
        let fixture = try Fixture(coreRunning: false)
        await fixture.viewModel.enableSystemProxy()
        let commands = fixture.commands
        let start = commands.firstIndex(of: "core start")
        let ready = commands.firstIndex(of: "groups ready")
        let enable = commands.firstIndex(of: "system-proxy on")
        try expect(start != nil && ready != nil && enable != nil, "Start saved core, read groups, enable proxy")
        try expect(start! < ready! && ready! < enable!, "Never enable proxy before the saved core is ready")
        try expect(fixture.viewModel.systemProxyEnabled && fixture.viewModel.toggleTestResult?.delay == 42,
                   "Verify real proxy status and test the selected group")
        try expect(!commands.contains { $0.hasPrefix("bootstrap") || $0.hasPrefix("subscription") },
                   "Recovery must not download or reimport the subscription")
    }

    @MainActor
    static func retriesTransientControllerFailures() async throws {
        let fixture = try Fixture()
        try fixture.set("transient-groups")
        await fixture.viewModel.enableSystemProxy()
        let commands = fixture.commands
        let enable = commands.firstIndex(of: "system-proxy on")!
        try expect(commands[..<enable].filter { $0 == "groups list --json" }.count == 3,
                   "Retry both failed and empty controller responses before enabling")
        try expect(fixture.viewModel.lastError == nil && fixture.viewModel.proxyGroupsError == nil,
                   "Clear temporary errors after recovery")
        try expect(!commands.contains("core start"), "Do not restart a healthy core")
    }

    @MainActor
    static func unavailableControllerNeverEnablesProxy() async throws {
        for flag in ["fail-groups", "empty-groups"] {
            let fixture = try Fixture()
            await fixture.viewModel.refreshStatus()
            try fixture.set(flag)
            await fixture.viewModel.enableSystemProxy()
            try expect(!fixture.has("proxy-active") && !fixture.commands.contains("system-proxy on"),
                       "Cached nodes must never establish readiness")
            try expect(fixture.viewModel.lastError?.contains("无需重新粘贴订阅") == true,
                       "Explain temporary readiness failures without requesting a new subscription")
            try expect(!fixture.viewModel.isToggleTesting, "Allow a later retry")
            try fixture.remove(flag)
            await fixture.viewModel.enableSystemProxy()
            try expect(fixture.viewModel.systemProxyEnabled && fixture.viewModel.lastError == nil,
                       "Recover on retry with the existing configuration")
        }
    }

    @MainActor
    static func refreshKeepsNodesUntilCoreStops() async throws {
        let fixture = try Fixture()
        await fixture.viewModel.refreshStatus()
        for flag in ["fail-groups", "empty-groups"] {
            try fixture.set(flag)
            await fixture.viewModel.refreshStatus()
            try expect(fixture.viewModel.proxyGroups.count == 1 && fixture.viewModel.currentNode == "Node A",
                       "Keep the node list and selection during transient controller failures")
            try expect(fixture.viewModel.proxyGroupsError != nil, "Mark cached nodes as temporarily unavailable")
            try fixture.remove(flag)
            await fixture.viewModel.refreshStatus()
            try expect(fixture.viewModel.proxyGroupsError == nil, "Clear stale-data warning after recovery")
        }
        try fixture.remove("core-active")
        await fixture.viewModel.refreshStatus()
        try expect(fixture.viewModel.proxyGroups.isEmpty && fixture.viewModel.currentGroup.isEmpty,
                   "Clear selections after the core actually stops")
    }

    @MainActor
    static func savedProfilesSurviveHelperFailure() async throws {
        let fixture = try Fixture()
        await fixture.viewModel.loadProfiles()
        try fixture.set("fail-profiles")
        await fixture.viewModel.loadProfiles()
        try expect(fixture.viewModel.profiles.map(\.id) == ["saved"] && fixture.viewModel.activeProfileID == "saved",
                   "Keep saved profiles and active selection when listing temporarily fails")
    }

    @MainActor
    static func failedCoreStartLeavesProxyOff() async throws {
        let fixture = try Fixture(coreRunning: false)
        try fixture.set("fail-start")
        await fixture.viewModel.enableSystemProxy()
        try expect(!fixture.commands.contains("system-proxy on") && !fixture.viewModel.coreRunning,
                   "A core start failure must not enable a dead proxy")
        try expect(fixture.viewModel.lastError?.contains("无法启动已保存的配置") == true,
                   "Report the actual saved-config startup error")
    }

    @MainActor
    static func campusArrivalCancelsRecovery() async throws {
        let fixture = try Fixture(coreRunning: false)
        try fixture.set("enter-campus")
        await fixture.viewModel.enableSystemProxy()
        try expect(!fixture.commands.contains("system-proxy on") && !fixture.has("core-active"),
                   "Arrival on protected campus Wi-Fi must cancel readiness and stop the newly started core")
        try expect(fixture.commands.suffix(2) == ["system-proxy off", "core stop"],
                   "Campus cancellation must clear proxy settings before stopping the core")
    }

    @MainActor
    static func enablingDuringRefreshPublishesRealProxyStatus() async throws {
        let fixture = try Fixture()
        try fixture.set("hold-mode")
        defer { try? fixture.remove("hold-mode") }
        let refresh = Task { await fixture.viewModel.refreshStatus() }
        for _ in 0..<200 where !fixture.has("mode-held") {
            try await Task.sleep(nanoseconds: 10_000_000)
        }
        try expect(fixture.has("mode-held"), "Periodic refresh must be in flight for this regression")
        await fixture.viewModel.enableSystemProxy()
        try expect(fixture.viewModel.isRefreshing && fixture.viewModel.systemProxyEnabled,
                   "Publish proxy status even when the periodic refresh skips overlapping work")
        try expect(fixture.viewModel.toggleTestResult?.delay == 42, "Still test fresh groups during a refresh")
        try fixture.remove("hold-mode")
        await refresh.value
    }

    @MainActor
    static func repeatedEnableDoesNotStartTwice() async throws {
        let fixture = try Fixture(coreRunning: false)
        try fixture.set("transient-groups")
        let first = Task { await fixture.viewModel.enableSystemProxy() }
        for _ in 0..<200 where !fixture.viewModel.isToggleTesting {
            try await Task.sleep(nanoseconds: 10_000_000)
        }
        await fixture.viewModel.enableSystemProxy()
        await first.value
        try expect(fixture.commands.filter { $0 == "core start" }.count == 1 &&
                   fixture.commands.filter { $0 == "system-proxy on" }.count == 1,
                   "Repeated clicks during recovery must not issue duplicate lifecycle commands")
    }

    @MainActor
    static func selectionAndDelayFollowMode() async throws {
        let fixture = try Fixture()
        try fixture.set("multi-groups")
        await fixture.viewModel.refreshStatus()
        try expect(fixture.viewModel.currentGroup == "Proxy" && fixture.viewModel.currentNode == "Auto",
                   "Rule mode must prefer its selector instead of the alphabetically first or GLOBAL group")
        for (mode, group, node, delay) in [("global", "GLOBAL", "Auto", 44), ("direct", "DIRECT", "DIRECT", 45), ("rule", "Proxy", "Auto", 43)] {
            fixture.viewModel.testResult = TestResult(googleOK: true, githubOK: true, error: nil)
            await fixture.viewModel.setMode(mode)
            try expect(fixture.viewModel.currentMode == mode && fixture.viewModel.currentGroup == group && fixture.viewModel.currentNode == node,
                       "Displayed selection must follow mode \(mode)")
            try expect(fixture.viewModel.toggleTestResult == nil && fixture.viewModel.testResult == nil,
                       "Changing modes must clear results from the previous route")
            await fixture.viewModel.enableSystemProxy()
            try expect(fixture.commands.contains("proxy-delay \(group) --json") && fixture.viewModel.toggleTestResult?.delay == delay,
                       "Enable test must use the actual \(mode) route")
        }
    }

    @MainActor
    static func failedModeChangeKeepsSelection() async throws {
        let fixture = try Fixture()
        await fixture.viewModel.refreshStatus()
        try fixture.set("fail-mode")
        await fixture.viewModel.setMode("global")
        try expect(fixture.viewModel.currentMode == "rule" && fixture.viewModel.currentGroup == "Auto" && fixture.viewModel.currentNode == "Node A",
                   "A failed global preparation must leave the displayed rule route intact")
        try expect(fixture.viewModel.lastError?.contains("global selection unavailable") == true && !fixture.viewModel.isChangingMode,
                   "Report failure and allow another mode change")
    }

    @MainActor
    static func reviewBeforeInstallingUpdate() async throws {
        let fixture = try Fixture()
        await fixture.viewModel.installUpdate()
        try expect(fixture.commands.isEmpty, "Never install without a checked version")
        await fixture.viewModel.checkForUpdate()
        try expect(fixture.viewModel.availableUpdateVersion == "0.13.0" && fixture.viewModel.updateReleaseNotes.contains("更新日志") && fixture.viewModel.updatePublishedAt != nil,
                   "Checking must preserve the notes, version and publication date for review")
        try expect(fixture.commands == ["self-update --check-only --json"] && !fixture.viewModel.showRestartAlert,
                   "Checking must not download or install")
        await fixture.viewModel.installUpdate()
        try expect(fixture.commands.last == "self-update --json --expected-version 0.13.0" && fixture.viewModel.showRestartAlert,
                   "Install the reviewed version and keep restart available in the window")
        try expect(fixture.viewModel.availableUpdateVersion == nil && !fixture.viewModel.isInstallingUpdate,
                   "Successful installation must not offer another install")
    }

    @MainActor
    static func interruptedUpdateIsRetryable() async throws {
        let fixture = try Fixture()
        await fixture.viewModel.checkForUpdate()
        try fixture.set("truncated-update")
        await fixture.viewModel.installUpdate()
        try expect(fixture.viewModel.updateFailed && !fixture.viewModel.showRestartAlert && fixture.viewModel.availableUpdateVersion == "0.13.0",
                   "An interrupted stream must allow a retry and must not show success")
    }

    @MainActor
    static func downloadNetworkFailureIsRetryable() async throws {
        let fixture = try Fixture()
        await fixture.viewModel.checkForUpdate()
        try fixture.set("network-update")
        await fixture.viewModel.installUpdate()
        try expect(fixture.viewModel.updateFailed && fixture.viewModel.updateStatus.contains("代理或 VPN") && fixture.viewModel.updateErrorDetails?.contains("i/o timeout") == true,
                   "Keep network recovery advice readable and retain technical details separately")
        try expect(fixture.viewModel.availableUpdateVersion == "0.13.0" && !fixture.viewModel.showRestartAlert,
                   "A failed download must allow retrying the reviewed version without claiming success")
        try fixture.remove("network-update")
        await fixture.viewModel.installUpdate()
        try expect(fixture.viewModel.showRestartAlert && !fixture.viewModel.updateFailed && fixture.viewModel.updateErrorDetails == nil,
                   "Retrying successfully must clear old errors and diagnostics")
    }

    @MainActor
    static func updateFailuresRemainReviewable() async throws {
        let fixture = try Fixture()
        try fixture.set("fail-check")
        await fixture.viewModel.checkForUpdate()
        try expect(fixture.viewModel.updateStatus == "更新检查暂时不可用，请稍后重试" && fixture.viewModel.updateFailed,
                   "Keep structured check errors instead of generic command exit failures")
        try expect(fixture.viewModel.showingCurrentReleaseNotes && fixture.viewModel.currentReleaseNotes.contains("离线更新日志") && fixture.viewModel.availableUpdateVersion == nil,
                   "A failed check must show bundled current-version notes without offering an install")
        try fixture.remove("fail-check")
        await fixture.viewModel.checkForUpdate()
        try expect(!fixture.viewModel.showingCurrentReleaseNotes && !fixture.viewModel.updateFailed && fixture.viewModel.updateReleaseNotes.contains("连接测试"),
                   "A successful retry must return to the available release's notes")
        try fixture.set("new-release")
        await fixture.viewModel.installUpdate()
        try expect(fixture.viewModel.availableUpdateVersion == nil && fixture.viewModel.updateFailed && !fixture.viewModel.showRestartAlert,
                   "A changed release must require a new check, not offer an unreviewed install")
    }

    @MainActor
    private static func waitForAutomaticUpdate(_ fixture: Fixture, count: Int = 1) async throws {
        for _ in 0..<300 {
            if fixture.updateCheckCount >= count && !fixture.viewModel.isCheckingForUpdate { return }
            try await Task.sleep(nanoseconds: 10_000_000)
        }
        throw TestFailure(description: "Automatic update check did not finish")
    }

    @MainActor
    static func automaticUpdatesRequireAnActiveConnection() async throws {
        let fixture = try Fixture(coreRunning: false, automaticUpdateChecks: true)
        try fixture.set("proxy-active")
        await fixture.viewModel.refreshStatus()
        try expect(fixture.updateCheckCount == 0, "A proxy switch without a running core must not trigger a check")
        try fixture.remove("proxy-active")
        try fixture.set("core-active")
        await fixture.viewModel.refreshStatus()
        try expect(fixture.updateCheckCount == 0, "An idle core with no system proxy or TUN must not trigger a check")
        try fixture.set("tun-active")
        await fixture.viewModel.refreshStatus()
        try await waitForAutomaticUpdate(fixture)
        try expect(fixture.viewModel.tunEnabled && !fixture.viewModel.systemProxyEnabled && fixture.viewModel.availableUpdateVersion == "0.13.0",
                   "A confirmed TUN connection must also trigger automatic discovery")
        try expect(!fixture.commands.contains(where: { $0.hasPrefix("self-update --json") }), "Automatic discovery must never install")
    }

    @MainActor
    static func automaticUpdatesPreserveReviewAndSurviveRelaunch() async throws {
        let fixture = try Fixture(automaticUpdateChecks: true)
        try fixture.set("proxy-active")
        fixture.viewModel.lastError = "Unrelated connection error"
        await fixture.viewModel.refreshStatus()
        try await waitForAutomaticUpdate(fixture)
        try expect(fixture.viewModel.availableUpdateVersion == "0.13.0" && fixture.viewModel.updateReleaseNotes.contains("更新日志"),
                   "Connecting the system proxy must discover a reviewable release")
        try expect(fixture.viewModel.lastError == "Unrelated connection error" && fixture.viewModel.showingCurrentReleaseNotes,
                   "Background discovery must preserve unrelated errors and the current changelog selection; error=\(String(describing: fixture.viewModel.lastError)), currentNotes=\(fixture.viewModel.showingCurrentReleaseNotes)")
        let reopened = StatusViewModel(helper: fixture.helper, defaults: fixture.defaults, wifiSSIDProvider: { nil }, currentVersion: "0.12.4")
        try expect(reopened.availableUpdateVersion == "0.13.0" && reopened.updateReleaseURL != nil && reopened.updateReleaseNotes.contains("更新日志"),
                   "Relaunch must retain a discovered version and its notes during the daily cooldown")
        await fixture.viewModel.refreshStatus()
        try expect(fixture.updateCheckCount == 1, "A pending update must not be repeatedly checked")
    }

    @MainActor
    static func backgroundSubscriptionUpdatesWaitForConnection() async throws {
        let fixture = try Fixture()
        try fixture.set("stale-maintenance")
        await fixture.viewModel.refreshStatus()
        try expect(!fixture.commands.contains("subscription update"), "Do not update subscriptions while the system proxy is off")

        try fixture.set("proxy-active")
        await fixture.viewModel.refreshStatus()
        for _ in 0..<300 {
            if fixture.commands.contains("subscription update") && !fixture.viewModel.isUpdatingSubscription { break }
            try await Task.sleep(nanoseconds: 10_000_000)
        }
        try expect(fixture.commands.filter { $0 == "subscription update" }.count == 1,
                   "An overdue subscription should update once after a confirmed proxy connection")
        try expect(fixture.viewModel.maintenanceStatus?.subscriptionLastSuccess == "2027-01-15T08:00:00Z",
                   "The app should refresh the success timestamp after its background update; status=\(String(describing: fixture.viewModel.maintenanceStatus))")
    }

    @MainActor
    private static func writeMaintenance(_ fixture: Fixture, attempted: Date, succeeded: Date, error: String? = nil, result: Bool = false) throws {
        let iso = ISO8601DateFormatter()
        var record: [String: String] = ["subscriptionLastAttempt": iso.string(from: attempted), "subscriptionLastSuccess": iso.string(from: succeeded)]
        if let error { record["subscriptionLastError"] = error }
        try JSONSerialization.data(withJSONObject: record).write(to: fixture.directory.appendingPathComponent(result ? "subscription-result-maintenance.json" : "maintenance.json"))
    }

    @MainActor
    private static func waitForSubscription(_ fixture: Fixture, count: Int) async throws {
        for _ in 0..<300 {
            if fixture.commands.filter({ $0 == "subscription update" }).count == count && !fixture.viewModel.isUpdatingSubscription { return }
            try await Task.sleep(nanoseconds: 10_000_000)
        }
        throw TestFailure(description: "Subscription update did not complete exactly \(count) times")
    }

    @MainActor
    static func backgroundSubscriptionRespectsDailyBoundaryAndRelaunch() async throws {
        let fixture = try Fixture()
        let lastSuccess = fixture.now
        try writeMaintenance(fixture, attempted: lastSuccess, succeeded: lastSuccess)
        try fixture.set("proxy-active")
        await fixture.viewModel.refreshStatus()
        fixture.now.addTimeInterval(24 * 60 * 60 - 1)
        await fixture.viewModel.refreshStatus()
        try expect(!fixture.commands.contains("subscription update"), "A successful subscription must not refresh before 24 hours")
        fixture.now.addTimeInterval(1)
        try writeMaintenance(fixture, attempted: fixture.now, succeeded: fixture.now, result: true)
        await fixture.viewModel.refreshStatus()
        try await waitForSubscription(fixture, count: 1)
        try expect(fixture.viewModel.maintenanceStatus?.subscriptionLastSuccess == ISO8601DateFormatter().string(from: fixture.now), "Background completion must publish the new success time")
        let reopened = StatusViewModel(helper: fixture.helper, defaults: fixture.defaults, wifiSSIDProvider: { nil }, currentVersion: "0.12.4", dateProvider: { fixture.now })
        await reopened.refreshStatus()
        try await Task.sleep(nanoseconds: 50_000_000)
        try expect(fixture.commands.filter { $0 == "subscription update" }.count == 1, "Reopening must honor persisted subscription cooldown")
    }

    @MainActor
    static func backgroundSubscriptionRetriesFailuresAfterThirtyMinutes() async throws {
        let fixture = try Fixture()
        let previousSuccess = fixture.now.addingTimeInterval(-2 * 24 * 60 * 60)
        try writeMaintenance(fixture, attempted: fixture.now, succeeded: previousSuccess, error: "HTTP 503")
        try fixture.set("proxy-active")
        try fixture.set("fail-subscription")
        fixture.now.addTimeInterval(30 * 60 - 1)
        await fixture.viewModel.refreshStatus()
        try expect(!fixture.commands.contains("subscription update"), "A failed subscription must wait 30 minutes before retrying")
        fixture.now.addTimeInterval(1)
        try writeMaintenance(fixture, attempted: fixture.now, succeeded: previousSuccess, error: "HTTP 503", result: true)
        await fixture.viewModel.refreshStatus()
        try await waitForSubscription(fixture, count: 1)
        try expect(fixture.viewModel.maintenanceStatus?.subscriptionLastSuccess == ISO8601DateFormatter().string(from: previousSuccess) && fixture.viewModel.maintenanceStatus?.subscriptionLastError == "HTTP 503", "Failure must preserve the last successful time and show its reason")
        try expect(fixture.viewModel.lastError?.contains("HTTP 503") == true, "A background failure must remain visible")
        fixture.now.addTimeInterval(30 * 60 - 1)
        await fixture.viewModel.refreshStatus()
        try expect(fixture.commands.filter { $0 == "subscription update" }.count == 1, "A failed retry must begin a new cooldown")
        fixture.now.addTimeInterval(1)
        try fixture.remove("fail-subscription")
        try writeMaintenance(fixture, attempted: fixture.now, succeeded: fixture.now, result: true)
        await fixture.viewModel.refreshStatus()
        try await waitForSubscription(fixture, count: 2)
        try expect(fixture.viewModel.maintenanceStatus?.subscriptionLastError == nil && fixture.viewModel.maintenanceStatus?.subscriptionLastSuccess == ISO8601DateFormatter().string(from: fixture.now), "A successful retry must clear the maintenance error and advance success time")
    }

    @MainActor
    static func backgroundSubscriptionDoesNotBlockOrOverlap() async throws {
        let fixture = try Fixture()
        try fixture.set("proxy-active")
        try fixture.set("stale-maintenance")
        try fixture.set("hold-subscription")
        defer { try? fixture.remove("hold-subscription") }
        await fixture.viewModel.refreshStatus()
        for _ in 0..<300 {
            if fixture.has("subscription-held") { break }
            try await Task.sleep(nanoseconds: 10_000_000)
        }
        try expect(fixture.has("subscription-held") && fixture.viewModel.isUpdatingSubscription, "The test must hold a real helper request in flight")
        await fixture.viewModel.refreshStatus()
        await fixture.viewModel.updateSubscription()
        try expect(fixture.commands.filter { $0 == "subscription update" }.count == 1 && fixture.viewModel.coreRunning && fixture.viewModel.systemProxyEnabled, "A held background update must allow refresh and reject duplicate updates")
        try expect(!fixture.commands.contains { $0 == "tun off" || $0 == "core restart" || $0 == "system-proxy off" }, "A background request must not disconnect the active capture")
        try fixture.remove("hold-subscription")
        try await waitForSubscription(fixture, count: 1)
    }

    @MainActor
    static func websitePreferencesReloadFromStorage() async throws {
        let fixture = try Fixture()
        try Data(#"{"preferences":[{"host":"example.com","route":"direct","createdAt":"2026-11-13T00:00:00Z"}]}"#.utf8).write(to: fixture.directory.appendingPathComponent("preferences.json"))
        await fixture.viewModel.loadWebsitePreferences()
        try expect(fixture.viewModel.websitePreference(for: "example.com")?.route == "direct", "A stored preference must load into the visible model")
        let reopened = StatusViewModel(helper: fixture.helper, defaults: fixture.defaults, wifiSSIDProvider: { nil }, currentVersion: "0.12.4")
        await reopened.loadWebsitePreferences()
        try expect(reopened.websitePreference(for: "https://example.com/path")?.route == "direct", "A fresh model must load the stored preference after reopening")
        try Data(#"{"preferences":[]}"#.utf8).write(to: fixture.directory.appendingPathComponent("preferences.json"))
        await reopened.loadWebsitePreferences()
        try expect(reopened.websitePreference(for: "example.com") == nil, "Restoring defaults must remove the visible preference badge on reload")
    }

    @MainActor
    static func automaticUpdateSuccessIsThrottled() async throws {
        let fixture = try Fixture(automaticUpdateChecks: true)
        try fixture.set("proxy-active")
        try fixture.set("no-update")
        await fixture.viewModel.refreshStatus()
        try await waitForAutomaticUpdate(fixture)
        try expect(fixture.viewModel.updateStatus.isEmpty && fixture.viewModel.availableUpdateVersion == nil,
                   "Already-current background results must not add noise to the menu")
        fixture.now.addTimeInterval(23 * 60 * 60)
        await fixture.viewModel.refreshStatus()
        let reopened = StatusViewModel(helper: fixture.helper, defaults: fixture.defaults, wifiSSIDProvider: { nil }, currentVersion: "0.12.4", dateProvider: { fixture.now })
        reopened.coreRunning = true
        reopened.systemProxyEnabled = true
        await reopened.checkForUpdate(automatically: true)
        try expect(fixture.updateCheckCount == 1, "The daily cooldown must survive refreshes and relaunches")
        fixture.now.addTimeInterval(60 * 60)
        await fixture.viewModel.refreshStatus()
        try await waitForAutomaticUpdate(fixture, count: 2)
        let upgraded = StatusViewModel(helper: fixture.helper, defaults: fixture.defaults, wifiSSIDProvider: { nil }, currentVersion: "0.12.5", dateProvider: { fixture.now })
        upgraded.coreRunning = true
        upgraded.systemProxyEnabled = true
        await upgraded.checkForUpdate(automatically: true)
        try expect(fixture.updateCheckCount == 3, "A newly installed version must get its own initial check")
    }

    @MainActor
    static func automaticUpdateFailuresAreSilentAndRetryable() async throws {
        let fixture = try Fixture(automaticUpdateChecks: true)
        try fixture.set("proxy-active")
        try fixture.set("fail-check")
        fixture.viewModel.lastError = "Unrelated connection error"
        await fixture.viewModel.refreshStatus()
        try await waitForAutomaticUpdate(fixture)
        try expect(fixture.viewModel.updateStatus.isEmpty && !fixture.viewModel.updateFailed && fixture.viewModel.lastError == "Unrelated connection error",
                   "Background failure must not create an error banner or erase connection errors")
        fixture.now.addTimeInterval(29 * 60)
        await fixture.viewModel.refreshStatus()
        try expect(fixture.updateCheckCount == 1, "Repeated status refreshes must not hammer a failing update service")
        fixture.now.addTimeInterval(60)
        try fixture.remove("fail-check")
        await fixture.viewModel.refreshStatus()
        try await waitForAutomaticUpdate(fixture, count: 2)
        try expect(fixture.viewModel.availableUpdateVersion == "0.13.0", "A failed check must retry after its shorter cooldown")
    }

    @MainActor
    static func automaticUpdateChecksDoNotOverlap() async throws {
        let fixture = try Fixture(automaticUpdateChecks: true)
        try fixture.set("proxy-active")
        fixture.viewModel.isInstallingUpdate = true
        await fixture.viewModel.refreshStatus()
        fixture.viewModel.isInstallingUpdate = false
        fixture.viewModel.showRestartAlert = true
        await fixture.viewModel.refreshStatus()
        try expect(fixture.updateCheckCount == 0, "Automatic checks must not interrupt installation or a pending restart")
        fixture.viewModel.showRestartAlert = false
        try fixture.set("hold-check")
        await fixture.viewModel.refreshStatus()
        for _ in 0..<300 {
            if fixture.updateCheckCount == 1 { break }
            try await Task.sleep(nanoseconds: 10_000_000)
        }
        try expect(fixture.updateCheckCount == 1 && fixture.viewModel.isCheckingForUpdateAutomatically, "Fixture must hold one background check in flight")
        await fixture.viewModel.checkForUpdate()
        await fixture.viewModel.refreshStatus()
        try expect(fixture.updateCheckCount == 1, "A manual request and refresh must share the existing in-flight check")
        try fixture.remove("hold-check")
        try await waitForAutomaticUpdate(fixture)
    }

    @MainActor
    static func manualUpdatesBypassAutomaticThrottling() async throws {
        let fixture = try Fixture(automaticUpdateChecks: true)
        try fixture.set("proxy-active")
        try fixture.set("no-update")
        await fixture.viewModel.checkForUpdate()
        try fixture.remove("no-update")
        await fixture.viewModel.refreshStatus()
        try expect(fixture.updateCheckCount == 1, "A successful manual check must also suppress unnecessary automatic checks")
        await fixture.viewModel.checkForUpdate()
        try expect(fixture.updateCheckCount == 2 && fixture.viewModel.availableUpdateVersion == "0.13.0",
                   "Manual checks must bypass the automatic cooldown")
        await fixture.viewModel.installUpdate()
        try expect(fixture.viewModel.showRestartAlert && fixture.defaults.data(forKey: "availableUpdate.0.12.4") == nil,
                   "Successful installation must clear the cached update notice")
    }

    @MainActor
    static func manualCheckCanJoinABackgroundFailure() async throws {
        let fixture = try Fixture(automaticUpdateChecks: true)
        try fixture.set("proxy-active")
        try fixture.set("hold-check")
        try fixture.set("fail-check")
        await fixture.viewModel.refreshStatus()
        for _ in 0..<300 {
            if fixture.updateCheckCount == 1 { break }
            try await Task.sleep(nanoseconds: 10_000_000)
        }
        try expect(fixture.viewModel.isCheckingForUpdateAutomatically, "Check must initially stay in the background")
        await fixture.viewModel.checkForUpdate()
        try expect(!fixture.viewModel.isCheckingForUpdateAutomatically, "An explicit request must make the existing result reviewable")
        try fixture.remove("hold-check")
        try await waitForAutomaticUpdate(fixture)
        try expect(fixture.updateCheckCount == 1 && fixture.viewModel.updateFailed && fixture.viewModel.updateStatus.contains("暂时不可用"),
                   "A user joining a background request must see its failure without a duplicate request")
    }

    static func legacyUpdateEventsStillDecode() throws {
        let event = try JSONDecoder().decode(UpdateEvent.self, from: Data(#"{"stage":"done","message":"已经是最新版"}"#.utf8))
        try expect(event.releaseNotes == nil && event.releaseVersion == nil, "Old helper responses remain compatible")
    }

    static func releaseNotesKeepSectionsAndContent() throws {
        let sections = ReleaseNotesSection.parse("## 新功能\n- **日志**：说明\n  补充内容\n## 修复\n- 连接测试\n\n## 安装\n打开 DMG。")
        try expect(sections.map(\.title) == ["新功能", "修复", "安装"] && sections[0].items == ["**日志**：说明 补充内容"] && sections[2].items == ["打开 DMG。"],
                   "Preserve headings, inline formatting and continuation lines")
        try expect(ReleaseNotesSection.parse("").isEmpty, "Missing notes must use the empty state")
    }

    @MainActor
    static func tunFollowsConfirmedStateAndCampusPolicy() async throws {
        let fixture = try Fixture()
        await fixture.viewModel.setTUNEnabled(true)
        try expect(fixture.viewModel.tunEnabled && !fixture.viewModel.systemProxyEnabled, "TUN works independently of system proxy")
        try expect(MenuBarProxyState(running: true, enabled: false, mode: "rule", tunEnabled: true) == .tun, "TUN alone must show active")
        try expect(MenuBarProxyState(running: true, enabled: false, mode: "direct", tunEnabled: true) == .tunDirect, "TUN direct mode remains distinct")
        try fixture.set("campus-arrived")
        await fixture.viewModel.refreshStatus()
        try expect(!fixture.viewModel.tunEnabled && !fixture.has("tun-active"), "Campus disconnect must clear TUN")
        let enables = fixture.commands.filter { $0 == "tun on --json" }.count
        await fixture.viewModel.setTUNEnabled(true)
        try expect(fixture.commands.filter { $0 == "tun on --json" }.count == enables, "Campus policy must block TUN authorization")
    }

    @MainActor
    static func tunFailureDoesNotClaimConnection() async throws {
        let fixture = try Fixture()
        try fixture.set("fail-tun")
        await fixture.viewModel.setTUNEnabled(true)
        try expect(!fixture.viewModel.tunEnabled && fixture.viewModel.lastError?.contains("取消") == true && !fixture.viewModel.isChangingTUN, "Cancelled authorization must not show success")
        try fixture.remove("fail-tun")
        try fixture.set("unready-tun")
        await fixture.viewModel.setTUNEnabled(true)
        try expect(!fixture.viewModel.tunEnabled && fixture.viewModel.lastError != nil, "Successful helper exit alone cannot certify TUN")
    }

    @MainActor
    static func completedTUNSwitchDoesNotWaitForNodeRefresh() async throws {
        let fixture = try Fixture()
        try fixture.set("hold-groups")
        let enable = Task { await fixture.viewModel.setTUNEnabled(true) }
        for _ in 0..<300 {
            if fixture.has("groups-held") { break }
            try await Task.sleep(nanoseconds: 10_000_000)
        }
        let reachedNodeRefresh = fixture.has("groups-held")
        let switchFinished = !fixture.viewModel.isChangingTUN
        await fixture.viewModel.setTUNEnabled(false)
        let disabledWhileRefreshing = fixture.commands.contains("tun off --json") && !fixture.has("tun-active")
        try fixture.remove("hold-groups")
        await enable.value
        try expect(reachedNodeRefresh, "Fixture must hold node refresh after TUN is confirmed")
        try expect(switchFinished, "A completed TUN operation must release the switch before unrelated node refresh")
        try expect(disabledWhileRefreshing && !fixture.viewModel.tunEnabled, "Slow node refresh must not prevent turning TUN off")
    }

    @MainActor
    static func unavailableCoreDiagnosisSurvivesPolling() async throws {
        let fixture = try Fixture(coreRunning: false)
        try fixture.set("fail-mode-status")
        try fixture.set("fail-groups")
        await fixture.viewModel.refreshStatus()
        try writeDiagnosis(diagnosticReport(), to: fixture)
        await fixture.viewModel.runDiagnose()
        let diagnosedAt = fixture.viewModel.diagnoseReport?.observedAt
        for _ in 0..<3 { await fixture.viewModel.refreshStatus() }
        try expect(fixture.viewModel.diagnoseReport?.observedAt == diagnosedAt && fixture.viewModel.diagnoseReport != nil,
                   "A stopped core and repeated controller failures must not make its diagnostic report disappear")
        try expect(!fixture.viewModel.diagnosisNeedsRefresh && fixture.viewModel.diagnosticActions == [.recoverCore],
                   "An unchanged unavailable core must keep its current diagnostic recovery button usable")
        try expect(!fixture.commands.contains("core start") && !fixture.commands.contains("system-proxy on"),
                   "Preserving diagnosis during polling must not silently repair or enable a proxy")
    }

    @MainActor
    static func failedDiagnosisKeepsLastReport() async throws {
        let fixture = try Fixture()
        await fixture.viewModel.refreshStatus()
        try writeDiagnosis(diagnosticReport(), to: fixture)
        await fixture.viewModel.runDiagnose()
        let previous = fixture.viewModel.diagnoseReport?.observedAt
        try fixture.set("fail-diagnose")
        await fixture.viewModel.runDiagnose()
        try expect(fixture.viewModel.diagnoseReport?.observedAt == previous && fixture.viewModel.diagnoseReport != nil,
                   "An unsuccessful recheck must keep the previous timestamped report readable")
        try expect(fixture.viewModel.diagnosisNeedsRefresh && fixture.viewModel.diagnosticActions.isEmpty && fixture.viewModel.lastError?.contains("诊断未完成") == true,
                   "A failed recheck must mark the historical result stale and suppress its repair actions")
    }

    @MainActor
    static func pendingDiagnosisKeepsLastReport() async throws {
        let fixture = try Fixture()
        await fixture.viewModel.refreshStatus()
        try writeDiagnosis(diagnosticReport(), to: fixture)
        await fixture.viewModel.runDiagnose()
        try fixture.set("hold-diagnose")
        let recheck = Task { await fixture.viewModel.runDiagnose() }
        for _ in 0..<300 {
            if fixture.has("diagnose-held") { break }
            try await Task.sleep(nanoseconds: 10_000_000)
        }
        let previousRemainsVisible = fixture.viewModel.diagnoseReport != nil && fixture.viewModel.isDiagnosing
        let recoveryBlocked = !fixture.viewModel.canPerformDiagnosticAction(.recoverCore)
        fixture.viewModel.clearDiagnosis()
        let pendingClearBlocked = fixture.viewModel.diagnoseReport != nil
        try fixture.remove("hold-diagnose")
        await recheck.value
        try expect(previousRemainsVisible && recoveryBlocked && pendingClearBlocked,
                   "A recheck must keep the last report visible and serialize clearing and recovery")
    }

    @MainActor
    static func diagnosisCanBeCleared() async throws {
        let fixture = try Fixture()
        await fixture.viewModel.refreshStatus()
        try writeDiagnosis(diagnosticReport(), to: fixture)
        await fixture.viewModel.runDiagnose()
        try fixture.set("fail-groups")
        await fixture.viewModel.refreshStatus()
        fixture.viewModel.clearDiagnosis()
        await fixture.viewModel.refreshStatus()
        try expect(fixture.viewModel.diagnoseReport == nil && !fixture.viewModel.diagnosisNeedsRefresh && fixture.viewModel.diagnosticActions.isEmpty,
                   "Only an explicit clear should remove the report; later polling must not resurrect it")
        await fixture.viewModel.runDiagnose()
        try expect(fixture.viewModel.diagnoseReport != nil && !fixture.viewModel.diagnosisNeedsRefresh,
                   "The user can create a fresh report after clearing history")
    }

    @MainActor
    static func networkChangesInvalidatePreviousDiagnosis() async throws {
        let fixture = try Fixture()
        await fixture.viewModel.refreshStatus()
        await fixture.viewModel.runDiagnose()
        try expect(fixture.viewModel.diagnoseReport != nil, "A fresh diagnosis must be visible")
        try fixture.set("fail-tun")
        await fixture.viewModel.setTUNEnabled(true)
        try expect(fixture.viewModel.diagnoseReport != nil && fixture.viewModel.diagnosisNeedsRefresh && fixture.viewModel.diagnosticActions.isEmpty,
                   "Even a failed switch must retain the old report only as history, without active recovery")
        try expect(fixture.viewModel.lastError != nil, "The current failure must remain visible")
        await fixture.viewModel.runDiagnose()
        try expect(fixture.viewModel.diagnoseReport != nil && !fixture.viewModel.diagnosisNeedsRefresh,
                   "An explicit new diagnosis may replace the stale notice")
        await fixture.viewModel.setMode("global")
        try expect(fixture.viewModel.diagnoseReport != nil && fixture.viewModel.diagnosisNeedsRefresh,
                   "A mode change must mark the old route's retained diagnosis as stale")
    }

    @MainActor
    static func lateDiagnosisCannotEraseSwitchFailure() async throws {
        let fixture = try Fixture()
        await fixture.viewModel.refreshStatus()
        try fixture.set("hold-diagnose")
        let diagnosis = Task { await fixture.viewModel.runDiagnose() }
        for _ in 0..<300 {
            if fixture.has("diagnose-held") { break }
            try await Task.sleep(nanoseconds: 10_000_000)
        }
        try fixture.set("fail-tun")
        await fixture.viewModel.setTUNEnabled(true)
        let switchError = fixture.viewModel.lastError
        try fixture.remove("hold-diagnose")
        await diagnosis.value
        try expect(switchError != nil && fixture.viewModel.lastError == switchError,
                   "A late all-pass diagnosis must not erase a newer switch failure")
        try expect(fixture.viewModel.diagnoseReport != nil && fixture.viewModel.diagnosisNeedsRefresh && fixture.viewModel.diagnosticActions.isEmpty,
                   "Late results remain readable history and cannot certify or repair the current network")
    }

    @MainActor
    static func observedChangesInvalidateDiagnosis() async throws {
        let fixture = try Fixture()
        await fixture.viewModel.refreshStatus()
        await fixture.viewModel.runDiagnose()
        try fixture.set("proxy-active")
        await fixture.viewModel.refreshStatus()
        try expect(fixture.viewModel.diagnoseReport != nil && fixture.viewModel.diagnosisNeedsRefresh,
                   "An externally observed proxy change must mark cached diagnosis as historical")
        await fixture.viewModel.runDiagnose()
        await fixture.viewModel.refreshStatus()
        try expect(fixture.viewModel.diagnoseReport != nil && !fixture.viewModel.diagnosisNeedsRefresh,
                   "An unchanged status refresh must preserve a fresh diagnosis")
    }

    @MainActor
    static func unreadableControllerInvalidatesDiagnosis() async throws {
        let fixture = try Fixture()
        await fixture.viewModel.refreshStatus()
        await fixture.viewModel.runDiagnose()
        try fixture.set("fail-groups")
        await fixture.viewModel.refreshStatus()
        try expect(fixture.viewModel.diagnoseReport != nil && fixture.viewModel.diagnosisNeedsRefresh && fixture.viewModel.diagnosticActions.isEmpty,
                   "A newly unavailable controller must retire all-pass but retain its timestamped report")
        try expect(!fixture.viewModel.proxyGroups.isEmpty && fixture.viewModel.proxyGroupsError != nil,
                   "Cached nodes remain useful while diagnostic success is retired")
    }

    private static func diagnosticReport(listener: String = "fail", action: String = "recoverCore", capture: String = "pass") throws -> DiagnoseReport {
        let json = #"{"app":"ProxyCat","milestone":"test","schemaVersion":2,"observedAt":"2026-11-11T08:00:00Z","scope":"仅检查本地链路和测试地址","checks":[{"name":"controller","category":"runtime","status":"pass","message":"ready"},{"name":"proxy-listener","category":"runtime","status":"\#(listener)","message":"listener result","action":"\#(action)"},{"name":"traffic-capture","category":"runtime","status":"\#(capture)","message":"capture result","action":"enableSystemProxy"},{"name":"https-request","category":"network","status":"\#(listener == "pass" ? "pass" : "skipped")","message":"network result"}]}"#
        return try JSONDecoder().decode(DiagnoseReport.self, from: Data(json.utf8))
    }

    @MainActor
    private static func writeDiagnosis(_ report: DiagnoseReport, to fixture: Fixture, file: String = "diagnose-result.json") throws {
        try JSONEncoder().encode(report).write(to: fixture.directory.appendingPathComponent(file))
    }

    static func diagnosticModelsDoNotOverstateOldOrIncompleteResults() throws {
        let old = try JSONDecoder().decode(DiagnoseReport.self, from: Data(#"{"app":"ProxyCat","milestone":"milestone-1","checks":[{"name":"files","status":"pass","message":"exists"}]}"#.utf8))
        try expect(!old.hasRuntimeEvidence && old.summary.contains("尚未验证") && old.timeLabel == "检测时间未知", "A legacy all-pass report cannot certify networking")
        let ready = try diagnosticReport(listener: "pass")
        try expect(ready.summary == "本地链路与测试地址通过" && ready.actions.isEmpty && ready.timeLabel.contains("11-11"), "A full report must name its limited scope and observation time")
        var incomplete = ready
        incomplete = DiagnoseReport(app: ready.app, milestone: ready.milestone, checks: ready.checks + [CheckResult(name: "new-check", status: "unknown", message: "unknown", suggestedFix: nil)], schemaVersion: 2)
        try expect(incomplete.summary == "部分项目未检查", "Unknown check statuses must not pass")
        let unknown = try diagnosticReport(action: "arbitraryShellCommand")
        try expect(unknown.actions.isEmpty, "Only known diagnostic actions may be offered")
        try expect(unknown.conclusion.contains("7890 未就绪"), "The main diagnostic result must explain the failed layer without opening details")
    }

    @MainActor
    static func diagnosticRecoveryAlwaysVerifiesTheListener() async throws {
        let fixture = try Fixture()
        await fixture.viewModel.refreshStatus()
        try writeDiagnosis(diagnosticReport(), to: fixture)
        await fixture.viewModel.runDiagnose()
        let before = fixture.commands
        try expect(!before.contains("core start") && !before.contains("system-proxy on"), "Diagnosis itself must be read-only")
        await fixture.viewModel.performDiagnosticAction(.recoverCore)
        try expect(fixture.commands.filter { $0 == "diagnose --json" }.count == 2 && fixture.commands.filter { $0 == "core start" }.count == 1, "Every repair must run the existing core operation then recheck")
        try expect(fixture.viewModel.diagnoseReport?.summary == "发现 1 项异常", "Helper success alone cannot certify a listener that still fails")
        try writeDiagnosis(diagnosticReport(listener: "pass"), to: fixture, file: "diagnose-after-start.json")
        await fixture.viewModel.performDiagnosticAction(.recoverCore)
        try expect(fixture.viewModel.diagnoseReport?.summary == "本地链路与测试地址通过", "Verified evidence may replace the failure")
        try expect(!fixture.commands.contains("system-proxy on") && !fixture.commands.contains(where: { $0.hasPrefix("tun ") || $0.hasPrefix("mode set") }), "Core recovery must not silently change capture or mode")
    }

    @MainActor
    static func diagnosticRecoveryPreservesFailures() async throws {
        let fixture = try Fixture()
        await fixture.viewModel.refreshStatus()
        try writeDiagnosis(diagnosticReport(), to: fixture)
        await fixture.viewModel.runDiagnose()
        try fixture.set("fail-start")
        await fixture.viewModel.performDiagnosticAction(.recoverCore)
        try expect(fixture.viewModel.lastError?.contains("saved config failed") == true && fixture.viewModel.diagnoseReport?.summary == "发现 1 项异常", "Post-repair diagnosis must preserve the command failure")
        try expect(!fixture.viewModel.isRecoveringDiagnosis && !fixture.viewModel.isDiagnosing, "Failures must release operation controls")
    }

    @MainActor
    static func diagnosticRecoveryGuardsTUNAndCampusPolicy() async throws {
        let fixture = try Fixture()
        await fixture.viewModel.refreshStatus()
        try writeDiagnosis(diagnosticReport(), to: fixture)
        await fixture.viewModel.runDiagnose()
        fixture.viewModel.tunEnabled = true
        await fixture.viewModel.performDiagnosticAction(.recoverCore)
        try expect(!fixture.commands.contains("core start"), "An active TUN must block ordinary-core repair even with a stale action")
        fixture.viewModel.tunEnabled = false
        try fixture.set("campus-arrived")
        await fixture.viewModel.performDiagnosticAction(.recoverCore)
        try expect(!fixture.commands.contains("core start") && fixture.viewModel.campusProtectionNotice != nil, "Repair must obey campus disconnect policy")
    }

    @MainActor
    static func diagnosticRecoveryDoesNotOverlapSwitches() async throws {
        let fixture = try Fixture()
        await fixture.viewModel.refreshStatus()
        try writeDiagnosis(diagnosticReport(), to: fixture)
        await fixture.viewModel.runDiagnose()
        try fixture.set("hold-start")
        let repair = Task { await fixture.viewModel.performDiagnosticAction(.recoverCore) }
        for _ in 0..<300 {
            if fixture.has("start-held") { break }
            try await Task.sleep(nanoseconds: 10_000_000)
        }
        let started = fixture.has("start-held") && fixture.viewModel.isRecoveringDiagnosis
        await fixture.viewModel.setTUNEnabled(true)
        await fixture.viewModel.setMode("global")
        await fixture.viewModel.performDiagnosticAction(.recoverCore)
        let during = fixture.commands
        try fixture.remove("hold-start")
        await repair.value
        try expect(started && during.filter { $0 == "core start" }.count == 1 && !during.contains("tun on --json") && !during.contains("mode set global"), "A pending repair must serialize UI operations and duplicate clicks")
        await fixture.viewModel.setMode("global")
        let count = fixture.commands.filter { $0 == "core start" }.count
        await fixture.viewModel.performDiagnosticAction(.recoverCore)
        try expect(fixture.commands.filter { $0 == "core start" }.count == count && fixture.viewModel.diagnosticActions.isEmpty, "A context change must retire diagnostic actions")
    }

    @MainActor
    static func diagnosticProxyEnableIsExplicitAndRechecked() async throws {
        let fixture = try Fixture()
        await fixture.viewModel.refreshStatus()
        try writeDiagnosis(diagnosticReport(listener: "pass", capture: "warn"), to: fixture)
        await fixture.viewModel.runDiagnose()
        try expect(!fixture.commands.contains("system-proxy on") && fixture.viewModel.diagnosticActions == [.enableSystemProxy], "An off proxy is explained without silently turning it on")
        await fixture.viewModel.performDiagnosticAction(.enableSystemProxy)
        try expect(fixture.has("proxy-active") && fixture.commands.filter { $0 == "diagnose --json" }.count == 2, "The explicitly requested capture action must be verified afterward")
    }

    @MainActor
    static func diagnosticResultsCannotEraseANewerError() async throws {
        let fixture = try Fixture()
        await fixture.viewModel.refreshStatus()
        try fixture.set("hold-diagnose")
        let diagnosis = Task { await fixture.viewModel.runDiagnose() }
        for _ in 0..<300 {
            if fixture.has("diagnose-held") { break }
            try await Task.sleep(nanoseconds: 10_000_000)
        }
        fixture.viewModel.lastError = "newer operation failed"
        try fixture.remove("hold-diagnose")
        await diagnosis.value
        try expect(fixture.viewModel.lastError == "newer operation failed", "Even an otherwise-current report must preserve newer operation errors")
    }

    @MainActor
    static func tunHelperRemovalRequiresInactiveTunnel() async throws {
        let fixture = try Fixture()
        fixture.viewModel.tunEnabled = true
        await fixture.viewModel.uninstallTUNHelper()
        try expect(!fixture.commands.contains("tun uninstall-helper"), "An active tunnel must block helper removal")
        fixture.viewModel.tunEnabled = false
        await fixture.viewModel.uninstallTUNHelper()
        try expect(fixture.commands.contains("tun uninstall-helper") && !fixture.viewModel.isChangingTUN, "Inactive helper removal must finish without touching normal proxy")
        try expect(!fixture.commands.contains("system-proxy off") && !fixture.commands.contains("core stop"), "Helper removal must preserve normal proxy")
    }

    @MainActor
    static func menuBarTracksProxyState() async throws {
        let fixture = try Fixture(coreRunning: false)
        var states: [MenuBarProxyState] = []
        let observer = MenuBarStatusObserver(viewModel: fixture.viewModel) { states.append($0.state) }
        try await Task.sleep(nanoseconds: 20_000_000)
        try expect(states.last == .off, "Menu bar starts with the system proxy off")

        await fixture.viewModel.enableSystemProxy()
        try await Task.sleep(nanoseconds: 20_000_000)
        try expect(states.last == .connected, "Enabling the proxy must update the menu bar without opening its window")
        await fixture.viewModel.setMode("direct")
        try await Task.sleep(nanoseconds: 20_000_000)
        try expect(states.last == .direct, "Direct mode must not look like an active proxy route")
        try fixture.remove("core-active")
        await fixture.viewModel.refreshStatus()
        try await Task.sleep(nanoseconds: 20_000_000)
        try expect(states.last == .engineStopped, "An enabled system proxy with a stopped core must warn")
        await fixture.viewModel.disableSystemProxy()
        try await Task.sleep(nanoseconds: 20_000_000)
        try expect(states.last == .off, "Disabling the system proxy must clear the active color")

        try fixture.set("core-active")
        await fixture.viewModel.refreshStatus()
        try await Task.sleep(nanoseconds: 20_000_000)
        try expect(states.last == .engineOnly, "A running core alone must not look connected")
        let count = states.count
        await fixture.viewModel.refreshStatus()
        try await Task.sleep(nanoseconds: 20_000_000)
        try expect(states.count == count, "Unchanged refreshes must not redraw the icon")
        for state in [MenuBarProxyState.off, .engineOnly, .connected, .direct, .engineStopped] {
            guard let image = state.image() else { throw TestFailure(description: "Missing status symbol") }
            try expect(image.isTemplate == (state.color == nil), "Active colors must survive macOS template rendering")
        }
        withExtendedLifetime(observer) {}
    }
}

extension ConnectionRecoveryTests {
    @MainActor
    private static func preferenceFixture(_ fixture: Fixture, state: String = "applied", route: String = "proxy", rule: String = "DomainSuffix", outcome: String = "responded") throws {
        try Data("{\"host\":\"example.com\",\"route\":\"\(route)\",\"message\":\"网站偏好已保存\",\"applicationState\":\"\(state)\"}".utf8)
            .write(to: fixture.directory.appendingPathComponent("preference-result.json"))
        var evidence = try JSONSerialization.jsonObject(with: websiteFixture(route, outcome: outcome)) as! [String: Any]
        evidence["rule"] = rule
        evidence["rulePayload"] = rule == "DomainSuffix" ? "example.com" : ""
        evidence["statusCode"] = outcome == "responded" ? 200 : 0
        try JSONSerialization.data(withJSONObject: evidence).write(to: fixture.directory.appendingPathComponent("website-result.json"))
    }

    static func websitePreferenceEvidenceRequiresTheSavedRule() throws {
        var evidence = try JSONSerialization.jsonObject(with: websiteFixture()) as! [String: Any]
        let match = try JSONDecoder().decode(WebsiteCheckResult.self, from: JSONSerialization.data(withJSONObject: evidence))
        try expect(!match.verifiesPreference(host: "example.com", route: "proxy"), "A MATCH proxy connection cannot verify the saved domain preference")
        evidence["rule"] = "DomainSuffix"; evidence["rulePayload"] = "example.com"
        let saved = try JSONDecoder().decode(WebsiteCheckResult.self, from: JSONSerialization.data(withJSONObject: evidence))
        try expect(saved.verifiesPreference(host: "https://EXAMPLE.COM.:8443/path", route: "proxy"), "Verification should normalize URL, port and trailing dot")
        try expect(!saved.verifiesPreference(host: "different.com", route: "proxy") && !saved.verifiesPreference(host: "example.com", route: "direct"), "A different host or route must not verify the requested preference")
        evidence["chains"] = [String]()
        let unknown = try JSONDecoder().decode(WebsiteCheckResult.self, from: JSONSerialization.data(withJSONObject: evidence))
        try expect(!unknown.verifiesPreference(host: "example.com", route: "proxy"), "Rule text without a connection chain is not route evidence")
    }

    @MainActor
    static func websitePreferenceAppliesAndVerifiesTheRequest() async throws {
        let fixture = try Fixture()
        await fixture.viewModel.refreshStatus()
        try preferenceFixture(fixture)
        await fixture.viewModel.checkWebsite("http://example.com:8080/specific-path")
        await fixture.viewModel.setWebsitePreference(host: "example.com", route: "proxy")
        try expect(fixture.viewModel.websitePreferenceStatus?.contains("已确认命中") == true && fixture.viewModel.websitePreferenceVerification?.node == "Node A", "Saving should verify the actual rule and node")
        try expect(fixture.commands.filter { $0 == "website-check http://example.com:8080/specific-path --json" }.count == 2, "Recheck should preserve the entered scheme, port and path")
        try expect(fixture.viewModel.websiteResultIsCurrent && !fixture.viewModel.isSavingWebsitePreference, "New evidence should replace the old website result and release the operation guard")
    }

    @MainActor
    static func websitePreferenceDoesNotCertifyUnknownRoutesOrFailures() async throws {
        let fixture = try Fixture()
        await fixture.viewModel.refreshStatus()
        try preferenceFixture(fixture, rule: "Match")
        await fixture.viewModel.setWebsitePreference(host: "example.com", route: "proxy")
        try expect(fixture.viewModel.websitePreferenceStatus?.contains("尚未验证命中") == true, "A matching route from MATCH must not be reported as preference success")
        try preferenceFixture(fixture, outcome: "timeout")
        await fixture.viewModel.setWebsitePreference(host: "example.com", route: "proxy")
        try expect(fixture.viewModel.websitePreferenceStatus?.contains("已确认命中") == true && fixture.viewModel.websitePreferenceStatus?.contains("超时") == true, "Confirmed routing must still explain a failed HTTP request")
        let checked = fixture.commands.filter { $0.hasPrefix("website-check") }.count
        try fixture.set("fail-preference")
        await fixture.viewModel.setWebsitePreference(host: "example.com", route: "direct")
        try expect(fixture.viewModel.websitePreferenceStatus?.contains("失败") == true && fixture.viewModel.websitePreferenceVerification == nil, "Application failure must retire previous verification")
        try expect(fixture.commands.filter { $0.hasPrefix("website-check") }.count == checked, "Failed application must not run a check against the old configuration")
    }

    @MainActor
    static func websitePreferenceDefersTUNAndNonRuleMode() async throws {
        let fixture = try Fixture()
        try fixture.set("tun-active")
        await fixture.viewModel.refreshStatus()
        try preferenceFixture(fixture, state: "pending")
        await fixture.viewModel.setWebsitePreference(host: "example.com", route: "proxy")
        try expect(!fixture.commands.contains(where: { $0.hasPrefix("website-check") || $0 == "tun off" || $0 == "core restart" }), "Saving a deferred TUN preference must not disrupt the active connection or certify the old rules")
        try fixture.remove("tun-active")
        try Data("global".utf8).write(to: fixture.directory.appendingPathComponent("mode"))
        try preferenceFixture(fixture)
        await fixture.viewModel.refreshStatus()
        await fixture.viewModel.setWebsitePreference(host: "example.com", route: "proxy")
        try expect(fixture.viewModel.websitePreferenceStatus?.contains("暂不参与分流") == true && !fixture.commands.contains(where: { $0.hasPrefix("website-check") }), "Global mode must explain the limited scope instead of claiming a preference match")
    }

    @MainActor
    static func websitePreferenceRemovalRechecksSubscriptionRoute() async throws {
        let fixture = try Fixture()
        await fixture.viewModel.refreshStatus()
        try preferenceFixture(fixture, route: "direct", rule: "Match")
        await fixture.viewModel.removeWebsitePreference(host: "example.com")
        try expect(fixture.commands.contains("preferences remove example.com --json") && fixture.viewModel.websitePreferenceStatus?.contains("已移除") == true && fixture.viewModel.websitePreferenceVerification?.route == "direct", "Restoring defaults should recheck and show the subscription route without claiming the removed rule still matched")
    }

    @MainActor
    static func websitePreferenceVerificationRejectsChangedContext() async throws {
        let fixture = try Fixture()
        await fixture.viewModel.refreshStatus()
        try preferenceFixture(fixture)
        try fixture.set("hold-website")
        let task = Task { await fixture.viewModel.setWebsitePreference(host: "example.com", route: "proxy") }
        for _ in 0..<300 { if fixture.has("website-held") { break }; try await Task.sleep(nanoseconds: 10_000_000) }
        try expect(fixture.has("website-held"), "Preference verification should reach the isolated request")
        try Data("direct".utf8).write(to: fixture.directory.appendingPathComponent("mode"))
        await fixture.viewModel.refreshStatus()
        try fixture.remove("hold-website")
        await task.value
        try expect(fixture.viewModel.websitePreferenceVerification == nil && fixture.viewModel.websitePreferenceStatus?.contains("发生变化") == true, "A late check from the previous mode must never certify current routing")
    }

    @MainActor
    static func websitePreferenceSerializesLifecycleControls() async throws {
        let fixture = try Fixture()
        await fixture.viewModel.refreshStatus()
        try preferenceFixture(fixture)
        try fixture.set("hold-preference")
        let task = Task { await fixture.viewModel.setWebsitePreference(host: "example.com", route: "proxy") }
        for _ in 0..<300 { if fixture.has("preference-held") { break }; try await Task.sleep(nanoseconds: 10_000_000) }
        try expect(fixture.has("preference-held") && fixture.viewModel.isSavingWebsitePreference, "The guard should cover application and verification")
        await fixture.viewModel.setWebsitePreference(host: "example.com", route: "direct")
        await fixture.viewModel.setTUNEnabled(true)
        await fixture.viewModel.setMode("global")
        await fixture.viewModel.updateSubscription()
        try expect(fixture.commands.filter { $0.hasPrefix("preferences set") }.count == 1 && !fixture.commands.contains("tun on --json") && !fixture.commands.contains("mode set global") && !fixture.commands.contains("subscription update"), "Repeated clicks, TUN, mode and subscriptions must not race the configuration transaction")
        try fixture.remove("hold-preference")
        await task.value
    }
}
