// Source: developer.huawei.com / openharmony Index.ets minimal sample
// Surface: @Entry + @Component + @State + build() + ArkUI chain + trailing block
// Strict-mode: explicit type on @State; no any/as

@Entry
@Component
struct Index {
  @State message: string = 'Hello World'

  build() {
    Row() {
      Column() {
        Text(this.message)
          .fontSize(50)
          .fontWeight(FontWeight.Bold)
      }
      .width('100%')
    }
    .height('100%')
  }
}
