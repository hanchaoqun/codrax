// Source: developer.huawei.com codelabs / @Builder decorator example
// Surface: @Builder + @BuilderParam + UI element reuse mechanism

@Component
struct CardComponent {
  @BuilderParam customHeader: () => void

  @Builder defaultHeader() {
    Text('Default Header').fontSize(20)
  }

  build() {
    Column() {
      if (this.customHeader) {
        this.customHeader()
      } else {
        this.defaultHeader()
      }
      Text('Body content')
        .fontSize(16)
    }
    .padding(16)
  }
}

@Builder function GlobalCard(title: string) {
  Column() {
    Text(title).fontSize(24)
  }
  .borderRadius(8)
}
