import Membrane

@objc(PhantomMembrane)
class PhantomMembrane: NSObject {

    @objc static func requiresMainQueueSetup() -> Bool {
        return false
    }

    @objc func start() {
        Membrane.MembraneStart()
    }
    
}
