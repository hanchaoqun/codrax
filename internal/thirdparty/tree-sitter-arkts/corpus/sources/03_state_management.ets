// Source: openharmony state management decorators
// Surface: @State, @Prop, @Link, @Provide, @Consume, @Watch, @Observed

@Observed
class UserModel {
  name: string = ''
  age: number = 0

  constructor(name: string, age: number) {
    this.name = name
    this.age = age
  }
}

@Component
struct ChildComponent {
  @Prop title: string
  @Link counter: number
  @Consume('theme') theme: string

  build() {
    Column() {
      Text(this.title)
      Button('Increment')
        .onClick(() => {
          this.counter++
        })
    }
  }
}

@Entry
@Component
struct ParentComponent {
  @State counter: number = 0
  @State user: UserModel = new UserModel('Alice', 30)
  @Provide('theme') theme: string = 'dark'

  @Watch('onCounterChange')
  onCounterChange(propName: string): void {
    console.info(`counter changed: ${this.counter}`)
  }

  build() {
    Column() {
      ChildComponent({ title: 'Counter', counter: $counter })
      Text(`Total: ${this.counter}`)
    }
  }
}
