// Source: openharmony @Styles and @Extend decorators
// Surface: @Styles function + @Extend(Type) attribute method chains

@Styles function commonCardStyle() {
  .width('100%')
  .padding(16)
  .borderRadius(8)
  .backgroundColor('#FFFFFF')
}

@Extend(Text) function highlight(color: string) {
  .fontColor(color)
  .fontWeight(FontWeight.Bold)
  .fontSize(18)
}

@Entry
@Component
struct StyledPage {
  build() {
    Column() {
      Text('Highlighted Text')
        .highlight('#FF0000')
      Column() {
        Text('Card Body')
      }
      .commonCardStyle()
    }
  }
}
