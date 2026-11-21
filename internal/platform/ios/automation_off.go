//go:build ios && !nsgoautomation

package ios

// automationBuild is set in `nsgo dev -automation`'s builds: surfaces keep their alerts for the automation to answer, and it acts on the running app's scenes through liveSurface.
const automationBuild = false
