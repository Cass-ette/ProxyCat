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
                if [ -f "$fixture_dir/website-result.json" ]; then cat "$fixture_dir/website-result.json"; else echo 'invalid website' >&2; exit 2; fi
                ;;
            'core status')
                active=false
                if [ -f "$fixture_dir/core-active" ]; then active=true; fi
                printf '{"running":%s,"pid":42}\n' "$active"
                ;;
            'diagnose --json')
                if [ -f "$fixture_dir/hold-diagnose" ]; then
                    touch "$fixture_dir/diagnose-held"
                    while [ -f "$fixture_dir/hold-diagnose" ]; do sleep 0.02; done
                fi
                echo '{"app":"ProxyCat","milestone":"test","checks":[{"name":"listener","status":"pass","message":"ready"}]}'
                ;;
            'core start')
                if [ -f "$fixture_dir/fail-start" ]; then
                    echo 'saved config failed to start' >&2
                    exit 7
                fi
                touch "$fixture_dir/core-active"
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
        try await networkChangesInvalidatePreviousDiagnosis()
        try await lateDiagnosisCannotEraseSwitchFailure()
        try await observedChangesInvalidateDiagnosis()
        try await unreadableControllerInvalidatesDiagnosis()
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
        print("Passed 51 connection, website, node, mode, TUN, menu bar, update and subscription traffic regression tests")
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
                   "Background discovery must preserve unrelated errors and the current changelog selection")
        let reopened = StatusViewModel(helper: fixture.helper, defaults: fixture.defaults, wifiSSIDProvider: { nil }, currentVersion: "0.12.4")
        try expect(reopened.availableUpdateVersion == "0.13.0" && reopened.updateReleaseURL != nil && reopened.updateReleaseNotes.contains("更新日志"),
                   "Relaunch must retain a discovered version and its notes during the daily cooldown")
        await fixture.viewModel.refreshStatus()
        try expect(fixture.updateCheckCount == 1, "A pending update must not be repeatedly checked")
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
    static func networkChangesInvalidatePreviousDiagnosis() async throws {
        let fixture = try Fixture()
        await fixture.viewModel.refreshStatus()
        await fixture.viewModel.runDiagnose()
        try expect(fixture.viewModel.diagnoseReport != nil, "A fresh diagnosis must be visible")
        try fixture.set("fail-tun")
        await fixture.viewModel.setTUNEnabled(true)
        try expect(fixture.viewModel.diagnoseReport == nil && fixture.viewModel.diagnosisNeedsRefresh,
                   "Even a failed switch must retire the old all-pass result")
        try expect(fixture.viewModel.lastError != nil, "The current failure must remain visible")
        await fixture.viewModel.runDiagnose()
        try expect(fixture.viewModel.diagnoseReport != nil && !fixture.viewModel.diagnosisNeedsRefresh,
                   "An explicit new diagnosis may replace the stale notice")
        await fixture.viewModel.setMode("global")
        try expect(fixture.viewModel.diagnoseReport == nil && fixture.viewModel.diagnosisNeedsRefresh,
                   "A mode change must invalidate the old route's diagnosis")
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
        try expect(fixture.viewModel.diagnoseReport == nil && fixture.viewModel.diagnosisNeedsRefresh,
                   "Results from before the switch cannot certify the current network")
    }

    @MainActor
    static func observedChangesInvalidateDiagnosis() async throws {
        let fixture = try Fixture()
        await fixture.viewModel.refreshStatus()
        await fixture.viewModel.runDiagnose()
        try fixture.set("proxy-active")
        await fixture.viewModel.refreshStatus()
        try expect(fixture.viewModel.diagnoseReport == nil && fixture.viewModel.diagnosisNeedsRefresh,
                   "An externally observed proxy change must invalidate cached diagnosis")
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
        try expect(fixture.viewModel.diagnoseReport == nil && fixture.viewModel.diagnosisNeedsRefresh,
                   "An unavailable controller must invalidate all-pass even if cached route fields stay unchanged")
        try expect(!fixture.viewModel.proxyGroups.isEmpty && fixture.viewModel.proxyGroupsError != nil,
                   "Cached nodes remain useful while diagnostic success is retired")
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
