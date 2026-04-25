// Source: openharmony list rendering ForEach / LazyForEach
// Surface: ForEach trailing block, LazyForEach with IDataSource

class BasicDataSource implements IDataSource {
  private listeners: DataChangeListener[] = []
  private originDataArray: string[] = []

  totalCount(): number {
    return this.originDataArray.length
  }

  getData(index: number): string {
    return this.originDataArray[index]
  }

  registerDataChangeListener(listener: DataChangeListener): void {
    if (this.listeners.indexOf(listener) < 0) {
      this.listeners.push(listener)
    }
  }

  unregisterDataChangeListener(listener: DataChangeListener): void {
    const pos = this.listeners.indexOf(listener)
    if (pos >= 0) {
      this.listeners.splice(pos, 1)
    }
  }
}

@Entry
@Component
struct ListPage {
  @State items: string[] = ['A', 'B', 'C', 'D']
  private dataSource: BasicDataSource = new BasicDataSource()

  build() {
    Column() {
      ForEach(this.items, (item: string, index: number) => {
        Text(`${index}: ${item}`)
          .fontSize(16)
      }, (item: string) => item)

      LazyForEach(this.dataSource, (item: string) => {
        ListItem() {
          Text(item)
        }
      }, (item: string) => item)
    }
  }
}
