export default interface Service<T, ID> {
    list(): Promise<T[]>
    get(id: ID): Promise<T>
    create(item: T): Promise<void>
    delete(id: ID): Promise<void>
    update(id: ID, item: T): Promise<void>
}
