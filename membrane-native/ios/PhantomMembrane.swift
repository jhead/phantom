import Membrane

@objc(PhantomMembrane)
class PhantomMembrane: NSObject {

    @objc static func requiresMainQueueSetup() -> Bool {
        return false
    }

    @objc func start() {
        Membrane.MembraneStart(PhantomDataPersistence.default)
    }
    
}

class PhantomDataPersistence : NSObject, MembraneNativePersistenceProtocol {
    
    private static let writingOptions = Data.WritingOptions.atomic
    private static let filename = "phantom.json"
    static let `default`: PhantomDataPersistence = PhantomDataPersistence()
    
    enum Error: Swift.Error {
        case fileAlreadyExists
        case invalidDirectory
        case writtingFailed
    }
    
    let fileManager: FileManager
    init(fileManager: FileManager = .default) {
        self.fileManager = fileManager
    }
    
    func readData(_ error: NSErrorPointer) -> String {
        let errorPointer = error

        do {
            return try read(fileNamed: PhantomDataPersistence.filename)
        } catch {
            errorPointer?.pointee = error as NSError
            return ""
        }
    }

    func storeData(_ data: String?) throws {
        if let dataBytes = data?.data(using: .utf8) {
            try save(
                fileNamed: PhantomDataPersistence.filename,
                data: dataBytes
            )
        }
    }
    
    private func save(fileNamed: String, data: Data) throws {
        guard let url = makeURL(forFileNamed: fileNamed) else {
            throw Error.invalidDirectory
        }
        
        do {
            if fileManager.fileExists(atPath: url.absoluteString) {
                try data.write(to: url, options: PhantomDataPersistence.writingOptions)
            } else {
                fileManager.createFile(atPath: url.absoluteString, contents: data)
            }
        } catch {
            debugPrint(error)
            throw Error.writtingFailed
        }
    }
    
    private func read(fileNamed: String) throws -> String {
        guard let url = makeURL(forFileNamed: fileNamed) else {
            throw Error.invalidDirectory
        }
        
        if fileManager.fileExists(atPath: url.absoluteString) {
            throw Error.fileAlreadyExists
        }
        
        do {
            return try String(contentsOf: url, encoding: .utf8)
        } catch {
            debugPrint(error)
            throw error
        }
    }
    
    private func makeURL(forFileNamed fileName: String) -> URL? {
        guard let url = fileManager
                .urls(for: .documentDirectory, in: .userDomainMask)
                .first else {
                    return nil
                }
        
        return url.appendingPathComponent(fileName)
    }
    
}
