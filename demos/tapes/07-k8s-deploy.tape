Output demos/gifs/k8s-deploy.gif
Set FontSize 14
Set Width 1200
Set Height 600
Set Theme "Catppuccin Mocha"
Set TypingSpeed 40ms
Set Padding 20

Type "# Kubernetes Deployment — Istio + Gateway API"
Enter
Sleep 1s

Type "# Validate all 4 overlays"
Enter
Sleep 500ms

Type "for o in local cloud on-prem air-gapped; do echo -n \"$o: \"; kubectl kustomize k8s/overlays/$o 2>/dev/null | grep -c '^kind:'; done"
Enter
Sleep 3s

Type "# Cloud overlay includes Istio mesh + Gateway API"
Enter
Sleep 500ms

Type "kubectl kustomize k8s/overlays/cloud | grep '^kind:' | sort | uniq -c | sort -rn | head -8"
Enter
Sleep 3s

Type "# Deploy to KinD (local)"
Enter
Sleep 500ms

Type "kubectl -n voiceagent get pods -o wide"
Enter
Sleep 3s

Type "# mTLS, circuit breaking, network policies — all included"
Enter
Sleep 2s
