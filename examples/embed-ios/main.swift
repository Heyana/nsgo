// An iOS app of its own that embeds the NSGo app in examples/embedded:
// its UIKit screen shows a status label, a button and two NSGoViews, each
// an app of its own. The host greets each view's user through its launch
// parameters, sends both a message from the button, answers their "time"
// calls, hears their "tapped" posts, routes their order links and gives
// each a badge of its own as a native component. README.md builds it.
import Embedded
import UIKit

/// The host's badge: a button showing the app's count, which emits "tap".
final class Badge: NSObject, NSGoComponent {
    let button = UIButton(type: .system)
    let context: NSGoComponentContext

    init(_ context: NSGoComponentContext) {
        self.context = context
        super.init()
        button.setTitle("Badge 0", for: .normal)
        button.backgroundColor = .secondarySystemFill
        button.layer.cornerRadius = 6
        button.addTarget(self, action: #selector(tapped), for: .touchUpInside)
    }

    var view: UIView { button }

    func setProp(_ name: String, value: Any?) {
        if name == "count" { button.setTitle("Badge \(value ?? 0)", for: .normal) }
    }

    @objc func tapped() { context.emit("tap", nil) }
}

final class HostController: UIViewController {
    let status = UILabel()
    var views: [NSGoView] = []
    var sent = 0

    override func viewDidLoad() {
        super.viewDidLoad()
        view.backgroundColor = .systemBackground
        status.text = "The host's own views are above; NSGo's below."
        status.font = .preferredFont(forTextStyle: .footnote)
        status.numberOfLines = 0
        let send = UIButton(type: .system)
        send.setTitle("Send a message", for: .normal)
        send.addTarget(self, action: #selector(sendMessage), for: .touchUpInside)
        send.setContentCompressionResistancePriority(.required, for: .horizontal)
        let bar = UIStackView(arrangedSubviews: [status, send])
        bar.spacing = 8

        for (user, accent) in [("Ada", nil), ("Grace", "#cf222e")] as [(String, String?)] {
            let app = NSGoView(frame: .zero)
            app.params = ["user": user]
            if let accent { app.tokens = ["accent": accent] }
            app.handle("time") { _, reply in
                reply(DateFormatter.localizedString(from: Date(), dateStyle: .none, timeStyle: .medium), nil)
            }
            app.handle("tapped") { [weak self] data, _ in
                let count = (data as? [String: Any])?["count"] ?? 0
                self?.status.text = "\(user) tapped \(count) times"
            }
            app.onOpenURL = { [weak self] url in
                // The host's own router takes its order pages.
                guard url.host == "example.com", url.path.hasPrefix("/orders/") else { return false }
                self?.status.text = "\(user) opened order \(url.lastPathComponent) in the host"
                return true
            }
            app.onError = { message in print("the app failed:", message) }
            app.register(component: "host.badge") { context in Badge(context) }
            views.append(app)
        }
        let apps = UIStackView(arrangedSubviews: views)
        apps.axis = .vertical
        apps.distribution = .fillEqually
        apps.spacing = 1
        let column = UIStackView(arrangedSubviews: [bar, apps])
        column.axis = .vertical
        column.spacing = 8
        column.translatesAutoresizingMaskIntoConstraints = false
        view.addSubview(column)
        let guide = view.safeAreaLayoutGuide
        NSLayoutConstraint.activate([
            column.leadingAnchor.constraint(equalTo: guide.leadingAnchor, constant: 12),
            column.trailingAnchor.constraint(equalTo: guide.trailingAnchor, constant: -12),
            column.topAnchor.constraint(equalTo: guide.topAnchor, constant: 8),
            column.bottomAnchor.constraint(equalTo: view.bottomAnchor),
        ])
    }

    @objc func sendMessage() {
        sent += 1
        for app in views { app.send("message", ["text": "Message \(sent) from the host"]) }
    }

    // The status bar the first app asks for.
    override var preferredStatusBarStyle: UIStatusBarStyle { views.first?.preferredStatusBarStyle ?? .default }
}

final class AppDelegate: UIResponder, UIApplicationDelegate {
    func application(_ application: UIApplication,
                     didFinishLaunchingWithOptions options: [UIApplication.LaunchOptionsKey: Any]?) -> Bool {
        NSGoStart()
        return true
    }
}

// Info.plist's UIApplicationSceneManifest names it.
@objc(HostSceneDelegate)
final class SceneDelegate: UIResponder, UIWindowSceneDelegate {
    var window: UIWindow?

    func scene(_ scene: UIScene, willConnectTo session: UISceneSession, options: UIScene.ConnectionOptions) {
        guard let scene = scene as? UIWindowScene else { return }
        window = UIWindow(windowScene: scene)
        window?.rootViewController = HostController()
        window?.makeKeyAndVisible()
    }
}

UIApplicationMain(CommandLine.argc, CommandLine.unsafeArgv, nil, NSStringFromClass(AppDelegate.self))
