// Import Wails runtime for proper bindings
import { App } from "../bindings/github.com/ibeckermayer/scroll4me";

async function login() {
  const btn = document.getElementById("login-btn");
  const originalText = btn.textContent;

  try {
    // Set loading state
    btn.disabled = true;
    btn.textContent = "Opening browser...";

    await App.TriggerLogin();

    // Refresh auth status after successful login
    await checkAuth();
  } catch (e) {
    console.error("Login failed:", e);
    alert("Login failed: " + e.message);
  } finally {
    // Restore button state
    btn.disabled = false;
    btn.textContent = originalText;
  }
}

async function saveSettings() {
  // TODO: Gather form data and call SaveConfig
  alert("Settings saved! (not yet implemented)");
}

async function loadSettings() {
  try {
    const config = await App.GetConfig();
    // TODO: Populate form with config values
    console.log("Config loaded:", config);
  } catch (e) {
    console.error("Failed to load config:", e);
  }
}

async function checkAuth() {
  try {
    const authenticated = await App.IsAuthenticated();
    const dot = document.getElementById("auth-status");
    const text = document.getElementById("auth-text");
    if (authenticated) {
      dot.classList.remove("disconnected");
      text.textContent = "Connected to X";
    } else {
      dot.classList.add("disconnected");
      text.textContent = "Not connected";
    }
  } catch (e) {
    console.error("Auth check failed:", e);
  }
}

// Initialize on load
document.addEventListener("DOMContentLoaded", () => {
  // Debug: log available Wails bindings
  console.log("Wails bindings loaded");

  // Wire up event handlers
  document.getElementById("login-btn").addEventListener("click", login);
  document.getElementById("save-btn").addEventListener("click", saveSettings);

  loadSettings();
  checkAuth();
});
